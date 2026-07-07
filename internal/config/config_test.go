package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	path := writeTemp(t, `
watch:
  local:
    enabled: true
  public:
    enabled: true
notifiers:
  - type: webhook
    config:
      url: https://example.com
`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Watch.Local.Interval.Duration() != 10*time.Second {
		t.Errorf("local interval = %v, want 10s", c.Watch.Local.Interval.Duration())
	}
	if c.Watch.Public.Interval.Duration() != 60*time.Second {
		t.Errorf("public interval = %v, want 60s", c.Watch.Public.Interval.Duration())
	}
	if len(c.Watch.Public.Sources) == 0 {
		t.Error("expected default public sources")
	}
}

func TestIntervalParsing(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want time.Duration
	}{
		{"duration string", "interval: 30s", 30 * time.Second},
		{"minutes", "interval: 1m30s", 90 * time.Second},
		{"bare int is seconds", "interval: 60", 60 * time.Second},
		{"omitted uses default", "", defaultLocalInterval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, `
watch:
  local:
    enabled: true
    `+tc.yaml+`
notifiers:
  - type: webhook
    config: {url: https://example.com}
`)
			c, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := c.Watch.Local.Interval.Duration(); got != tc.want {
				t.Errorf("interval = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInvalidDuration(t *testing.T) {
	path := writeTemp(t, `
watch:
  local: {enabled: true, interval: "notaduration"}
notifiers:
  - type: webhook
    config: {url: https://example.com}
`)
	if _, err := Load(path); err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestGatewayDefault(t *testing.T) {
	path := writeTemp(t, `
watch:
  local: {enabled: true}
gateway:
  enabled: true
notifiers:
  - type: webhook
    config: {url: https://example.com}
`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Gateway.Listen != "127.0.0.1:8555" {
		t.Errorf("gateway listen = %q, want default", c.Gateway.Listen)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	if DefaultConfigPath() == "" {
		t.Error("DefaultConfigPath returned empty")
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]string{
		"no watcher": `
watch:
  local: {enabled: false}
  public: {enabled: false}
notifiers:
  - type: webhook
    config: {url: x}
`,
		"no notifier": `
watch:
  local: {enabled: true}
notifiers: []
`,
		"notifier missing type": `
watch:
  local: {enabled: true}
notifiers:
  - config: {url: x}
`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeTemp(t, content)); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestSourcesForRegion(t *testing.T) {
	cn := sourcesForRegion("cn")
	if len(cn) == 0 || cn[0] != "https://myip.ipip.net" {
		t.Errorf("cn region should use the China list, got %v", cn)
	}
	g := sourcesForRegion("global")
	if len(g) == 0 || g[0] != "https://api.ipify.org" {
		t.Errorf("global region should use the international list, got %v", g)
	}
}

func TestPublicRegionAppliedAsDefault(t *testing.T) {
	c := &Config{}
	c.Watch.Public.Enabled = true
	c.Watch.Public.Region = "cn"
	c.applyDefaults()
	if len(c.Watch.Public.Sources) == 0 || c.Watch.Public.Sources[0] != "https://myip.ipip.net" {
		t.Errorf("region=cn should seed China sources, got %v", c.Watch.Public.Sources)
	}
	// An explicit Sources list must win over region.
	c2 := &Config{}
	c2.Watch.Public.Enabled = true
	c2.Watch.Public.Region = "cn"
	c2.Watch.Public.Sources = []string{"https://example.com/ip"}
	c2.applyDefaults()
	if len(c2.Watch.Public.Sources) != 1 || c2.Watch.Public.Sources[0] != "https://example.com/ip" {
		t.Errorf("explicit sources should override region, got %v", c2.Watch.Public.Sources)
	}
}
