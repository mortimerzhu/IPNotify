// Package config loads and validates the YAML configuration for IPNotify.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
type Config struct {
	Watch     WatchConfig      `yaml:"watch"`
	Gateway   GatewayConfig    `yaml:"gateway"`
	Notifiers []NotifierConfig `yaml:"notifiers"`
}

// GatewayConfig configures the built-in HTTP status/control server.
type GatewayConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"` // host:port, defaults to 127.0.0.1:8555
}

// Duration wraps time.Duration with YAML support. It accepts either a Go
// duration string ("10s", "1m30s") or a bare number, which is interpreted as
// seconds (so `interval: 60` means 60 seconds). Plain time.Duration cannot be
// unmarshalled by yaml.v3, hence this type.
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	// A bare number (yaml.v3 would happily decode it into a string too, so we
	// branch on the node tag) is treated as seconds.
	switch node.Tag {
	case "!!int", "!!float":
		var n float64
		if err := node.Decode(&n); err != nil {
			return fmt.Errorf("invalid duration %q", node.Value)
		}
		*d = Duration(time.Duration(n * float64(time.Second)))
		return nil
	default:
		var s string
		if err := node.Decode(&s); err != nil {
			return fmt.Errorf("invalid duration %q", node.Value)
		}
		if s == "" {
			*d = 0
			return nil
		}
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// WatchConfig groups the watcher settings.
type WatchConfig struct {
	Local  LocalWatchConfig  `yaml:"local"`
	Public PublicWatchConfig `yaml:"public"`
}

// LocalWatchConfig configures the LAN IP watcher.
type LocalWatchConfig struct {
	Enabled bool `yaml:"enabled"`
	// Interval is the poll interval on platforms without native change events.
	Interval Duration `yaml:"interval"`
	// Interfaces optionally restricts monitoring to these interface names.
	// Empty means all non-loopback interfaces.
	Interfaces []string `yaml:"interfaces"`
	// DisableIPv6 drops all IPv6 addresses when true.
	DisableIPv6 bool `yaml:"disable_ipv6"`
	// IncludeIPv6ULA includes IPv6 unique-local addresses (fc00::/7). These are
	// auto-generated and noisy, so they are excluded by default.
	IncludeIPv6ULA bool `yaml:"include_ipv6_ula"`
	// IncludeVirtual includes virtual interfaces (VPN/bridge/docker/...). By
	// default only physical wired/Wi-Fi interfaces are reported. Ignored when
	// Interfaces is set.
	IncludeVirtual bool `yaml:"include_virtual"`
}

// PublicWatchConfig configures the public egress IP watcher.
type PublicWatchConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Interval Duration `yaml:"interval"`
	// Region selects the default source list when Sources is empty:
	//   "cn"     - mainland-China echo services (reachable directly even when a
	//              transparent proxy like OpenClash proxies foreign traffic)
	//   "global" - international echo services (ipify/ifconfig.me/icanhazip)
	//   "auto"/"" - pick by system timezone (China zone -> cn, else global)
	// An explicit Sources list always overrides this.
	Region string `yaml:"region"`
	// Sources are HTTP endpoints that echo the caller's public IP. The IP is
	// extracted from the response body by regex, so services that wrap it in
	// text/HTML (e.g. myip.ipip.net) work too.
	Sources []string `yaml:"sources"`
}

// NotifierConfig is one notifier entry. Config is passed opaquely to the
// matching notifier factory, keeping this package decoupled from concrete
// notifier implementations (open/closed principle).
type NotifierConfig struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config"`
}

// Default intervals used when a watcher is enabled but no interval is set.
const (
	defaultLocalInterval  = 10 * time.Second
	defaultPublicInterval = 60 * time.Second
	defaultGatewayListen  = "127.0.0.1:8555"
)

// globalPublicSources are international echo services, used when region is
// "global" (or auto-detected as non-China) and no explicit sources are set.
var globalPublicSources = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
}

// chinaPublicSources are mainland-China echo services. They resolve to the real
// broadband egress IP even when a transparent proxy (e.g. OpenClash) proxies
// foreign traffic but direct-routes Chinese destinations. Several wrap the IP in
// text/HTML, so the watcher extracts it by regex.
var chinaPublicSources = []string{
	"https://myip.ipip.net",
	"https://ddns.oray.com/checkip",
	"https://ip.3322.net",
	"https://4.ipw.cn",
	"https://v4.yinghualuo.cn/bejson",
}

// sourcesForRegion returns the default source list for a region ("cn"/"global",
// or "auto"/"" to pick by system timezone).
func sourcesForRegion(region string) []string {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "cn", "china":
		return append([]string(nil), chinaPublicSources...)
	case "global", "intl", "international":
		return append([]string(nil), globalPublicSources...)
	default: // "auto" / ""
		if detectChinaTimezone() {
			return append([]string(nil), chinaPublicSources...)
		}
		return append([]string(nil), globalPublicSources...)
	}
}

// detectChinaTimezone reports whether the system timezone looks like mainland
// China. Best-effort and offline: checks $TZ, /etc/timezone, and the current
// zone abbreviation/offset (CST +08:00).
func detectChinaTimezone() bool {
	tz := os.Getenv("TZ")
	if tz == "" {
		if b, err := os.ReadFile("/etc/timezone"); err == nil {
			tz = string(b)
		}
	}
	tz = strings.ToLower(strings.TrimSpace(tz))
	for _, z := range []string{"shanghai", "chongqing", "urumqi", "harbin", "asia/beijing", "prc"} {
		if strings.Contains(tz, z) {
			return true
		}
	}
	// CST at +08:00 is China Standard Time (US "CST" is -06:00).
	if name, off := time.Now().Zone(); strings.EqualFold(name, "CST") && off == 8*3600 {
		return true
	}
	return false
}

// DefaultConfigPath returns the conventional config file location for the
// current OS. The interactive installer writes to the same paths.
func DefaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("ProgramData")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return programData + `\IPNotify\config.yaml`
	case "darwin":
		// Per-user LaunchAgent: keep config in the user's home so no sudo is
		// needed and the user-run service can read it.
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library/Application Support/IPNotify/config.yaml")
	default: // linux, openwrt, and other unix-likes
		return "/etc/ipnotify/config.yaml"
	}
}

// Load reads, parses, validates, and applies defaults to the config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Watch.Local.Enabled && c.Watch.Local.Interval <= 0 {
		c.Watch.Local.Interval = Duration(defaultLocalInterval)
	}
	if c.Watch.Public.Enabled {
		if c.Watch.Public.Interval <= 0 {
			c.Watch.Public.Interval = Duration(defaultPublicInterval)
		}
		if len(c.Watch.Public.Sources) == 0 {
			c.Watch.Public.Sources = sourcesForRegion(c.Watch.Public.Region)
		}
	}
	if c.Gateway.Enabled && c.Gateway.Listen == "" {
		c.Gateway.Listen = defaultGatewayListen
	}
}

func (c *Config) validate() error {
	if !c.Watch.Local.Enabled && !c.Watch.Public.Enabled {
		return fmt.Errorf("config: at least one watcher (local or public) must be enabled")
	}
	if len(c.Notifiers) == 0 {
		return fmt.Errorf("config: at least one notifier must be configured")
	}
	for i, n := range c.Notifiers {
		if n.Type == "" {
			return fmt.Errorf("config: notifiers[%d] missing type", i)
		}
	}
	return nil
}
