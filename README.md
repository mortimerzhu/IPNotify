# IPNotify

> Monitor local/public IP changes and send notifications via your favorite IM platforms.

IPNotify is a lightweight, extensible IP monitoring tool written in Go.

It watches local or public IP address changes and sends notifications through multiple messaging platforms, making it suitable for home servers, cloud instances, NAS devices, edge devices, and developer workstations.

## Features

- 🚀 Lightweight and standalone
- 🔍 Monitor local IP changes
- 🌍 Monitor public IP changes (Coming Soon)
- 📡 Real-time monitoring (Netlink on Linux, polling on other platforms)
- 🔔 Multiple notification channels
  - DingTalk
  - Feishu
  - WeCom
  - Slack
  - Telegram
  - Discord
  - Email
  - Webhook
- 🔌 Pluggable notifier architecture
- ⚙️ Simple YAML configuration
- 🖥 Cross-platform (Linux / macOS / Windows)

## Architecture

```
          +-------------+
          |   Watcher   |
          +------+------+ 
                 |
         IP Changed Event
                 |
         +-------+--------+
         |                |
     Local IP        Public IP
                 |
          +------+------+
          | Notification |
          +------+------+
                 |
    +------------+-------------------------------+
    | DingTalk | Feishu | WeCom | Slack | ... |
    +--------------------------------------------+
```

## Roadmap

- [x] Local IP monitoring (cross-platform polling)
- [x] Public IP monitoring (HTTP multi-source)
- [x] DingTalk notifier
- [x] Feishu notifier
- [ ] WeCom notifier
- [ ] Slack notifier
- [x] Telegram notifier
- [ ] Discord notifier
- [ ] Email notifier
- [x] Webhook notifier
- [x] Background service (systemd / launchd / procd / Windows SCM)
- [x] Interactive installer script
- [x] HTTP status gateway
- [x] Notifier hot reload (`/reload`)
- [ ] Linux netlink real-time watcher
- [ ] Docker image
- [ ] Homebrew
- [ ] Prometheus metrics

## Why IPNotify?

Many existing tools focus on DDNS updates after an IP change.

IPNotify focuses on **notification** instead.

Receive instant alerts when your IP changes through your preferred messaging platform.

## Installation

### Quick install — prebuilt binary (recommended, no Go needed)

Detects your OS/arch, downloads the matching binary from the latest
[GitHub release](https://github.com/mortimerzhu/IPNotify/releases), verifies its
checksum, then runs the interactive installer (which registers a **background
service that starts on boot**).

```bash
# Linux / macOS / OpenWrt
curl -fsSL https://raw.githubusercontent.com/mortimerzhu/IPNotify/main/install.sh | sh

# Windows (run in an elevated PowerShell)
irm https://raw.githubusercontent.com/mortimerzhu/IPNotify/main/install.ps1 | iex
```

Useful environment variables:

| Variable                | Effect                                              |
|-------------------------|-----------------------------------------------------|
| `IPNOTIFY_VERSION`      | Install a specific release (e.g. `v0.0.1`) not the latest |
| `IPNOTIFY_NO_INSTALL=1` | Only download + verify the binary; skip installing   |
| `IPNOTIFY_REPO`         | Pull from a fork (`owner/name`)                     |

Prebuilt targets: `linux` (amd64/arm64/armv7), `darwin` (amd64/arm64),
`windows` (amd64/arm64). On other architectures (e.g. MIPS routers), install
from source below.

### Install from source

The interactive installer builds from source, prompts you for your watchers and
notifier credentials (webhook URLs, tokens, secrets), writes the config, and
installs IPNotify as a **background service** that starts on boot. Requires Go
1.26+ on the machine (or a prebuilt binary — see below).

```bash
git clone https://github.com/mortimerzhu/IPNotify.git
cd IPNotify

# Linux (systemd) / macOS (launchd) / OpenWrt (procd)
./scripts/install.sh

# Windows (run in an elevated PowerShell)
powershell -ExecutionPolicy Bypass -File scripts\install.ps1
```

No Go on the target (e.g. an OpenWrt router)? Cross-compile elsewhere and point
the installer at the binary:

```bash
GOOS=linux GOARCH=arm64 go build -o ipnotify ./cmd/ipnotify
IPNOTIFY_BINARY=./ipnotify ./scripts/install.sh
```

Uninstall: `./scripts/uninstall.sh` (add `PURGE=1` to also remove binary + config).

### Manual build

```bash
go build -o ipnotify ./cmd/ipnotify
```

## Usage

IPNotify runs as a system service. The same binary manages its own service
across systemd, launchd, OpenWrt procd, and the Windows SCM:

```bash
ipnotify service install     # register the service
ipnotify service start       # start it
ipnotify service status      # running / stopped
ipnotify service restart
ipnotify service stop
ipnotify service uninstall

ipnotify test                # send a test notification to all notifiers
ipnotify run                 # run in the foreground (SIGINT/SIGTERM to stop)
ipnotify version
```

On **Linux/OpenWrt/Windows** IPNotify installs as a system service, so service
commands need root/admin (`sudo ipnotify service ...`). On **macOS** it installs
as a per-user LaunchAgent — no `sudo`, and it runs in your login session so the
loopback gateway is reachable.

`-c` selects the config file; it defaults to the OS-specific path:

| OS            | Default config path                                    |
|---------------|--------------------------------------------------------|
| Linux/OpenWrt | `/etc/ipnotify/config.yaml`                            |
| macOS         | `~/Library/Application Support/IPNotify/config.yaml`   |
| Windows       | `%ProgramData%\IPNotify\config.yaml`                   |

Check service logs the usual way: `journalctl -u ipnotify -f` (systemd),
`log show --predicate 'process == "ipnotify"'` (macOS), `logread -e ipnotify`
(OpenWrt), or the Windows Event Viewer.

## Gateway (HTTP status API)

When `gateway.enabled: true`, IPNotify serves a small control API (default
`127.0.0.1:8555`, loopback-only):

| Method | Path       | Purpose                                             |
|--------|------------|-----------------------------------------------------|
| GET    | `/healthz` | Liveness probe (`200 ok`)                           |
| GET    | `/status`  | Current IPs, uptime, last change & last-notify info |
| POST   | `/test`    | Send a test notification to all notifiers           |
| POST   | `/reload`  | Reload notifiers from the config file               |

**Linux / macOS (bash) or Windows `cmd.exe`** — `curl` is the real curl:

```bash
curl http://127.0.0.1:8555/status          # GET
curl -X POST http://127.0.0.1:8555/test    # POST — send a test notification
curl -X POST http://127.0.0.1:8555/reload  # POST — reload notifiers
```

**Windows PowerShell** — bare `curl` is an alias for `Invoke-WebRequest` (defaults to
GET and does not accept curl flags like `-X`), so use `curl.exe` or the native cmdlet:

```powershell
# real curl (note the .exe)
curl.exe http://127.0.0.1:8555/status
curl.exe -X POST http://127.0.0.1:8555/test

# or the native PowerShell cmdlet (auto-parses JSON)
Invoke-RestMethod http://127.0.0.1:8555/status
Invoke-RestMethod -Method POST http://127.0.0.1:8555/test
```

> GET endpoints (`/healthz`, `/status`) work with a plain browser or GET request.
> `/test`, `/reload` are **POST-only** — a GET returns `405 method not allowed`.

> `/reload` hot-swaps the notifier set only; changing watcher intervals or the
> gateway listen address requires `ipnotify service restart`.

## Configuration

Configuration is YAML. See [`configs/config.example.yaml`](configs/config.example.yaml)
for a fully commented example.

```yaml
watch:
  local:                     # LAN interface IP monitoring
    enabled: true
    interval: 10s            # poll interval; a bare number (10) means seconds
    # By default only physical wired/Wi-Fi interfaces are watched; VPN tunnels,
    # bridges, docker/veth, VM adapters, loopback, link-local and IPv6 ULA are
    # auto-excluded — so you don't need to know per-OS interface names.
    # interfaces: [en0, en1]  # optional: watch ONLY these (overrides auto-detect)
    # include_virtual: false  # true = also watch VPN/bridge/docker interfaces
    # disable_ipv6: false     # true = IPv4 only
    # include_ipv6_ula: false # true = also report fd00::/7 (excluded by default)
  public:                    # public egress IP via HTTP services
    enabled: true
    interval: 60s
    # sources: [...]         # optional; sensible defaults are used

gateway:                     # built-in HTTP status/control API
  enabled: true
  listen: "127.0.0.1:8555"

notifiers:                   # one or more; each has a type + type-specific config
  - type: webhook
    config:
      url: https://example.com/hook
      headers: { Authorization: "Bearer TOKEN" }
  - type: telegram
    config: { token: "123:ABC", chat_id: "123456789" }
  - type: dingtalk
    config: { webhook: "https://oapi.dingtalk.com/robot/send?access_token=...", secret: "SEC..." }
  - type: feishu
    config: { webhook: "https://open.feishu.cn/open-apis/bot/v2/hook/...", secret: "..." }
```

At least one watcher and one notifier must be configured. DingTalk and Feishu
`secret` fields are optional and enable HMAC-SHA256 request signing when set.

### Extending

Add a new channel by implementing `notifier.Notifier` in a sub-package under
`pkg/notifier/` and calling `notifier.Register("<type>", New)` from its `init()`.
No changes to the core engine are needed.

## License

MIT
