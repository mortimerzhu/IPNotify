// Command ipnotify is a background service that watches local and public IP
// changes and notifies subscribers over IM platforms.
//
// Usage:
//
//	ipnotify [run]                                  run in foreground (default)
//	ipnotify test                                   send a test notification and exit
//	ipnotify service install|uninstall|start|stop|restart|status
//	ipnotify version
//
// The -c flag selects the config file (default: OS-specific config path).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/kardianos/service"

	"github.com/mortimerzhu/IPNotify/internal/app"
	"github.com/mortimerzhu/IPNotify/internal/config"

	// Register notifier implementations via their init() functions.
	_ "github.com/mortimerzhu/IPNotify/pkg/notifier/dingtalk"
	_ "github.com/mortimerzhu/IPNotify/pkg/notifier/feishu"
	_ "github.com/mortimerzhu/IPNotify/pkg/notifier/telegram"
	_ "github.com/mortimerzhu/IPNotify/pkg/notifier/webhook"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// First positional arg selects the subcommand (default: run).
	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "run":
		runService(log, args)
	case "test":
		runTest(log, args)
	case "validate":
		runValidate(log, args)
	case "service":
		runControl(log, args)
	case "version", "-v", "--version":
		fmt.Println("ipnotify", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

// parseConfigFlag parses a flagset exposing -c and returns the config path.
func parseConfigFlag(name string, args []string) string {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	cfgPath := fs.String("c", config.DefaultConfigPath(), "path to config file")
	_ = fs.Parse(args)
	return *cfgPath
}

func runService(log *slog.Logger, args []string) {
	cfgPath := parseConfigFlag("run", args)
	svc, err := app.NewService(cfgPath, log)
	if err != nil {
		log.Error("failed to init service", "err", err)
		os.Exit(1)
	}
	// service.Run handles interactive (foreground) and service-managed modes:
	// it calls program.Start, waits for termination, then program.Stop.
	if err := svc.Run(); err != nil {
		log.Error("service exited", "err", err)
		os.Exit(1)
	}
}

func runTest(log *slog.Logger, args []string) {
	cfgPath := parseConfigFlag("test", args)
	results, err := app.RunTest(context.Background(), cfgPath, log)
	if err != nil {
		log.Error("test failed", "err", err)
		os.Exit(1)
	}
	failed := 0
	for _, r := range results {
		if r.Success {
			fmt.Printf("  ✅ %s\n", r.Notifier)
		} else {
			failed++
			fmt.Printf("  ❌ %s: %s\n", r.Notifier, r.Error)
		}
	}
	fmt.Printf("%d/%d notifiers OK\n", len(results)-failed, len(results))
	if failed > 0 {
		os.Exit(1)
	}
}

func runValidate(log *slog.Logger, args []string) {
	cfgPath := parseConfigFlag("validate", args)
	if _, err := config.Load(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("config OK: %s\n", cfgPath)
}

func runControl(log *slog.Logger, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ipnotify service <install|uninstall|start|stop|restart|status> [-c config]")
		os.Exit(2)
	}
	action := args[0]
	cfgPath := parseConfigFlag("service", args[1:])
	svc, err := app.NewService(cfgPath, log)
	if err != nil {
		log.Error("failed to init service", "err", err)
		os.Exit(1)
	}

	if action == "status" {
		status, err := svc.Status()
		if err != nil {
			log.Error("failed to query status", "err", err)
			os.Exit(1)
		}
		fmt.Println("ipnotify:", statusString(status))
		return
	}

	if err := service.Control(svc, action); err != nil {
		log.Error("service control failed", "action", action, "err", err)
		os.Exit(1)
	}
	fmt.Printf("ipnotify %s: ok\n", action)
}

func statusString(s service.Status) string {
	switch s {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

func usage() {
	fmt.Print(`ipnotify - monitor local/public IP changes and notify via IM

Usage:
  ipnotify [run] [-c config]     Run in the foreground (default)
  ipnotify test [-c config]      Send a test notification to all notifiers
  ipnotify validate [-c config]  Parse and validate the config file
  ipnotify service <action>      Manage the system service
                                 actions: install uninstall start stop restart status
  ipnotify version               Print version

Default config path: ` + config.DefaultConfigPath() + "\n")
}
