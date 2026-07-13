// Package app assembles watchers, notifiers, engine, and gateway from config,
// and wraps them as a cross-platform system service via kardianos/service.
package app

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/kardianos/service"

	"github.com/mortimerzhu/IPNotify/internal/config"
	"github.com/mortimerzhu/IPNotify/internal/gateway"
	"github.com/mortimerzhu/IPNotify/pkg/ipnotify"
	"github.com/mortimerzhu/IPNotify/pkg/notifier"
	"github.com/mortimerzhu/IPNotify/pkg/watcher"
	"github.com/mortimerzhu/IPNotify/pkg/watcher/local"
	"github.com/mortimerzhu/IPNotify/pkg/watcher/public"
)

// localConfig maps the config's local-watch settings to the watcher's Config.
func localConfig(cfg *config.Config) local.Config {
	return local.Config{
		Interval:       cfg.Watch.Local.Interval.Duration(),
		Interfaces:     cfg.Watch.Local.Interfaces,
		DisableIPv6:    cfg.Watch.Local.DisableIPv6,
		IncludeIPv6ULA: cfg.Watch.Local.IncludeIPv6ULA,
		IncludeVirtual: cfg.Watch.Local.IncludeVirtual,
	}
}

// BuildWatchers constructs the enabled watchers from config.
func BuildWatchers(cfg *config.Config) []watcher.Watcher {
	var ws []watcher.Watcher
	if cfg.Watch.Local.Enabled {
		ws = append(ws, local.New(localConfig(cfg)))
	}
	if cfg.Watch.Public.Enabled {
		ws = append(ws, public.New(public.Config{
			Interval: cfg.Watch.Public.Interval.Duration(),
			Sources:  cfg.Watch.Public.Sources,
		}))
	}
	return ws
}

// BuildNotifiers constructs notifiers from config via the notifier registry.
func BuildNotifiers(cfg *config.Config) ([]notifier.Notifier, error) {
	var ns []notifier.Notifier
	for _, nc := range cfg.Notifiers {
		n, err := notifier.Build(nc.Type, nc.Config)
		if err != nil {
			return nil, err
		}
		ns = append(ns, n)
	}
	return ns, nil
}

// Run loads config, wires the engine and (optionally) the gateway, and blocks
// until ctx is cancelled.
func Run(ctx context.Context, configPath string, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	notifiers, err := BuildNotifiers(cfg)
	if err != nil {
		return err
	}
	watchers := BuildWatchers(cfg)
	opts := append([]ipnotify.Option{ipnotify.WithLogger(log)}, ipProviderOptions(cfg)...)
	engine := ipnotify.New(watchers, notifiers, opts...)
	log.Info("ipnotify starting",
		"watchers", len(watchers), "notifiers", len(notifiers), "gateway", cfg.Gateway.Enabled)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = engine.Run(ctx)
	}()

	// Announce the current IPs once at startup so a reboot notifies even when
	// the IP did not change. Runs in its own goroutine because the public-IP
	// provider does a live (blocking) lookup.
	if cfg.NotifyOnStartEnabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			engine.AnnounceStartup(ctx)
		}()
	}

	if cfg.Gateway.Enabled {
		reload := func() error {
			ncfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			ns, err := BuildNotifiers(ncfg)
			if err != nil {
				return err
			}
			engine.SetNotifiers(ns)
			log.Info("notifiers reloaded", "count", len(ns))
			return nil
		}
		gw := gateway.New(engine, cfg.Gateway.Listen, reload, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := gw.Run(ctx); err != nil {
				log.Error("gateway error", "err", err)
			}
		}()
	}

	wg.Wait()
	return ctx.Err()
}

// RunTest loads config, builds notifiers, and sends a synthetic notification to
// each, returning per-notifier results.
func RunTest(ctx context.Context, configPath string, log *slog.Logger) ([]ipnotify.TestResult, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	notifiers, err := BuildNotifiers(cfg)
	if err != nil {
		return nil, err
	}
	opts := append([]ipnotify.Option{ipnotify.WithLogger(log)}, ipProviderOptions(cfg)...)
	engine := ipnotify.New(nil, notifiers, opts...)
	return engine.TestAll(ctx), nil
}

// ipProviderOptions wires a test-notification IP provider for each ENABLED
// watcher only, so `ipnotify test` (RunTest) and the gateway /test endpoint
// (Run) return the same, config-accurate result: local only, WAN only, or both.
func ipProviderOptions(cfg *config.Config) []ipnotify.Option {
	var opts []ipnotify.Option
	if cfg.Watch.Local.Enabled {
		lc := localConfig(cfg)
		opts = append(opts, ipnotify.WithLocalIPs(func() []string { return local.CurrentIPs(lc) }))
	}
	if cfg.Watch.Public.Enabled {
		sources := cfg.Watch.Public.Sources
		opts = append(opts, ipnotify.WithPublicIPs(func() []string {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			return public.CurrentIP(ctx, sources)
		}))
	}
	return opts
}

// program implements service.Interface for kardianos/service.
type program struct {
	configPath string
	log        *slog.Logger
	cancel     context.CancelFunc
	done       chan struct{}
}

// Start is non-blocking per the kardianos contract: it launches Run in a
// goroutine and returns.
func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		if err := Run(ctx, p.configPath, p.log); err != nil && err != context.Canceled {
			p.log.Error("service run error", "err", err)
		}
	}()
	return nil
}

// Stop cancels the run context and waits (bounded) for a clean shutdown.
func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.done != nil {
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
		}
	}
	return nil
}

// ServiceConfig is the kardianos service definition shared across commands.
func ServiceConfig(configPath string) *service.Config {
	c := &service.Config{
		Name:        "ipnotify",
		DisplayName: "IPNotify",
		Description: "Monitor local/public IP changes and notify subscribers via IM.",
		Arguments:   []string{"run", "-c", configPath},
	}
	// On macOS install as a per-user LaunchAgent (no sudo needed to manage, runs
	// in the user session so the loopback gateway is reachable). Linux/OpenWrt
	// (systemd/procd) and Windows (SCM) use a system service. All commands build
	// the config the same way, so install/start/status/uninstall stay consistent.
	if runtime.GOOS == "darwin" {
		// RunAtLoad launches the agent as soon as it's loaded at login/boot
		// instead of relying solely on KeepAlive to resurrect it, so IP changes
		// that happen during a reboot are caught by the startup announce.
		c.Option = service.KeyValue{"UserService": true, "RunAtLoad": true}
	}
	return c
}

// NewService builds a kardianos service bound to the program and config path.
func NewService(configPath string, log *slog.Logger) (service.Service, error) {
	prg := &program{configPath: configPath, log: log}
	return service.New(prg, ServiceConfig(configPath))
}
