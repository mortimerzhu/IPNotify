#!/bin/sh
# IPNotify interactive installer.
#   Linux  -> systemd system service (needs root)
#   OpenWrt-> procd system service   (needs root)
#   macOS  -> per-user LaunchAgent   (no sudo; only the binary copy may prompt)
#
# Usage:
#   ./scripts/install.sh
#   IPNOTIFY_BINARY=/path/to/ipnotify ./scripts/install.sh   # skip build
#
# Non-interactive: preset prompts via env vars and ASSUME_YES=1
#   LOCAL_ENABLED LOCAL_INTERVAL PUBLIC_ENABLED PUBLIC_INTERVAL
#   GATEWAY_ENABLED GATEWAY_LISTEN
set -eu

err()  { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }

ask() { # ask "prompt" "default" VARNAME
  _p=$1; _d=$2
  if [ "${ASSUME_YES:-0}" = "1" ]; then eval "$3=\$_d"; return; fi
  if [ -n "$_d" ]; then printf '%s [%s]: ' "$_p" "$_d" >&2; else printf '%s: ' "$_p" >&2; fi
  if [ -r /dev/tty ]; then IFS= read -r _a < /dev/tty || _a=""; else IFS= read -r _a || _a=""; fi
  [ -z "$_a" ] && _a=$_d
  eval "$3=\$_a"
}
ask_yn() { ask "$1 (y/n)" "$2" "$3"; }
yesbool() { [ "$1" = "y" ] && echo true || echo false; }

# ---------- detect platform + privilege model ----------
OS=$(uname -s 2>/dev/null || echo unknown)
IS_OPENWRT=0; [ -f /etc/openwrt_release ] && IS_OPENWRT=1

# USER_SERVICE=1 -> per-user service, no privilege escalation for service/config.
case "$OS" in
  Darwin)
    USER_SERVICE=1
    BIN_DIR=/usr/local/bin
    CONFIG_DIR="$HOME/Library/Application Support/IPNotify" ;;
  Linux)
    USER_SERVICE=0
    [ "$IS_OPENWRT" = "1" ] && BIN_DIR=/usr/bin || BIN_DIR=/usr/local/bin
    CONFIG_DIR=/etc/ipnotify ;;
  *) err "unsupported OS: $OS (use install.ps1 on Windows)" ;;
esac
CONFIG_PATH="$CONFIG_DIR/config.yaml"

# SVC: privilege for service control + config write. BIN: for the binary copy.
if [ "$USER_SERVICE" = "1" ]; then
  SVC=""            # user LaunchAgent + user-owned config: no sudo
else
  if [ "$(id -u)" = "0" ]; then SVC=""; elif command -v sudo >/dev/null 2>&1; then SVC="sudo"; else
    err "root required for a system service; re-run as root or install sudo"
  fi
fi
# Binary lives in a system dir; use sudo only if it's not writable.
if [ -w "$BIN_DIR" ] || [ "$(id -u)" = "0" ]; then BIN=""; elif command -v sudo >/dev/null 2>&1; then BIN="sudo"; else BIN=""; fi

SCRIPT_DIR=$(cd "$(dirname "$0")" 2>/dev/null && pwd || echo .)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." 2>/dev/null && pwd || echo .)
TMP=$(mktemp -d 2>/dev/null || echo "/tmp/ipnotify.$$"); mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT

info "Platform: $OS (openwrt=$IS_OPENWRT, user-service=$USER_SERVICE)"
info "Binary: $BIN_DIR/ipnotify   Config: $CONFIG_PATH"

# ---------- obtain binary ----------
if [ -n "${IPNOTIFY_BINARY:-}" ]; then
  [ -x "$IPNOTIFY_BINARY" ] || err "IPNOTIFY_BINARY not executable: $IPNOTIFY_BINARY"
  SRC_BIN="$IPNOTIFY_BINARY"; info "Using prebuilt binary: $SRC_BIN"
elif command -v go >/dev/null 2>&1; then
  [ -f "$REPO_ROOT/go.mod" ] || err "go.mod not found in $REPO_ROOT; run from a cloned repo or set IPNOTIFY_BINARY"
  info "Building with $(go version | awk '{print $3}') ..."
  ( cd "$REPO_ROOT" && go build -o "$TMP/ipnotify" ./cmd/ipnotify )
  SRC_BIN="$TMP/ipnotify"
else
  err "Go not found and IPNOTIFY_BINARY unset.
Cross-compile elsewhere and re-run, e.g.:
  GOOS=linux GOARCH=arm64 go build -o ipnotify ./cmd/ipnotify
  IPNOTIFY_BINARY=./ipnotify ./scripts/install.sh"
fi

# ---------- seed defaults from an existing config ----------
# On re-install, reuse the previous answers as prompt defaults (press Enter to
# keep) so the same details don't have to be retyped. Precedence for each
# setting: explicit env var > existing config value > hard-coded default.
# Parser is deliberately minimal: it only understands the fixed shape this
# installer emits (env-var presets or a hand-edited config still override it).
read_existing_config() {
  CFG_LOCAL_ENABLED=""; CFG_LOCAL_INTERVAL=""; CFG_PUBLIC_ENABLED=""; CFG_PUBLIC_INTERVAL=""
  CFG_GATEWAY_ENABLED=""; CFG_GATEWAY_LISTEN=""; CFG_NOTIFIERS=""; CFG_NOTIFIER_COUNT=0
  [ -f "$CONFIG_PATH" ] || return 0

  # Scalar watcher/gateway settings. `top`/`sub2` track the section so the two
  # enabled:/interval: keys under watch.local vs watch.public don't collide.
  eval "$(awk '
    function val(s){ sub(/^[^:]*:[ \t]*/,"",s); sub(/[ \t]+#.*$/,"",s); sub(/[ \t]+$/,"",s); gsub(/^"|"$/,"",s); return s }
    /^[^ \t#]/ {
      line=$0; sub(/[ \t]+$/,"",line)
      if (line ~ /^watch:/) top="watch"; else if (line ~ /^gateway:/) top="gateway"; else top=""
      sub2=""; next
    }
    top=="watch" && /^[ \t]+local:/  { sub2="local";  next }
    top=="watch" && /^[ \t]+public:/ { sub2="public"; next }
    top=="watch" && /^[ \t]+enabled:/ { v=(val($0)=="true")?"y":"n"
      if (sub2=="local")  print "CFG_LOCAL_ENABLED=\"" v "\""
      if (sub2=="public") print "CFG_PUBLIC_ENABLED=\"" v "\""; next }
    top=="watch" && /^[ \t]+interval:/ { v=val($0)
      if (sub2=="local")  print "CFG_LOCAL_INTERVAL=\"" v "\""
      if (sub2=="public") print "CFG_PUBLIC_INTERVAL=\"" v "\""; next }
    top=="gateway" && /^[ \t]+enabled:/ { v=(val($0)=="true")?"y":"n"; print "CFG_GATEWAY_ENABLED=\"" v "\""; next }
    top=="gateway" && /^[ \t]+listen:/  { print "CFG_GATEWAY_LISTEN=\"" val($0) "\""; next }
  ' "$CONFIG_PATH")"

  # Notifiers block verbatim (line after `notifiers:` to the next top-level key
  # or EOF), with leading blank lines trimmed. Lets a re-install keep webhook
  # URLs / secrets / tokens without re-typing them.
  CFG_NOTIFIERS=$(awk '/^notifiers:/{inb=1;next} inb && /^[^ \t#]/{inb=0} inb{print}' "$CONFIG_PATH" | sed -e '/./,$!d')
  CFG_NOTIFIER_COUNT=$(printf '%s\n' "$CFG_NOTIFIERS" | grep -c '^[[:space:]]*-[[:space:]]*type:' || true)
}
read_existing_config
if [ -n "$CFG_LOCAL_ENABLED$CFG_PUBLIC_ENABLED$CFG_GATEWAY_LISTEN$CFG_NOTIFIERS" ]; then
  info "Found existing config — using its values as defaults (press Enter to keep)"
fi

# ---------- interactive config ----------
info "Configure watchers"
ask_yn "Enable local (LAN) IP watcher?" "${LOCAL_ENABLED:-${CFG_LOCAL_ENABLED:-y}}" LOCAL_ENABLED
LOCAL_INTERVAL="${LOCAL_INTERVAL:-${CFG_LOCAL_INTERVAL:-10}}"
[ "$LOCAL_ENABLED" = "y" ] && ask "  local poll interval (seconds)" "$LOCAL_INTERVAL" LOCAL_INTERVAL

ask_yn "Enable public (egress) IP watcher?" "${PUBLIC_ENABLED:-${CFG_PUBLIC_ENABLED:-y}}" PUBLIC_ENABLED
PUBLIC_INTERVAL="${PUBLIC_INTERVAL:-${CFG_PUBLIC_INTERVAL:-60}}"
[ "$PUBLIC_ENABLED" = "y" ] && ask "  public poll interval (seconds)" "$PUBLIC_INTERVAL" PUBLIC_INTERVAL

[ "$LOCAL_ENABLED" = "y" ] || [ "$PUBLIC_ENABLED" = "y" ] || err "at least one watcher must be enabled"

info "Configure gateway (HTTP status/control API)"
ask_yn "Enable gateway?" "${GATEWAY_ENABLED:-${CFG_GATEWAY_ENABLED:-y}}" GATEWAY_ENABLED
GATEWAY_LISTEN="${GATEWAY_LISTEN:-${CFG_GATEWAY_LISTEN:-127.0.0.1:8555}}"
[ "$GATEWAY_ENABLED" = "y" ] && ask "  gateway listen address" "$GATEWAY_LISTEN" GATEWAY_LISTEN

NOTIFIERS=""
add_notifier() {
  ask "Notifier type (dingtalk/feishu/telegram/webhook)" "" NTYPE
  case "$NTYPE" in
    dingtalk)
      ask "  DingTalk webhook URL" "" V1
      ask "  DingTalk secret (加签; blank to skip signing)" "" V2
      NOTIFIERS="$NOTIFIERS
  - type: dingtalk
    config:
      webhook: \"$V1\""
      [ -n "$V2" ] && NOTIFIERS="$NOTIFIERS
      secret: \"$V2\"" ;;
    feishu)
      ask "  Feishu webhook URL" "" V1
      ask "  Feishu secret (blank to skip signing)" "" V2
      NOTIFIERS="$NOTIFIERS
  - type: feishu
    config:
      webhook: \"$V1\""
      [ -n "$V2" ] && NOTIFIERS="$NOTIFIERS
      secret: \"$V2\"" ;;
    telegram)
      ask "  Telegram bot token" "" V1
      ask "  Telegram chat_id" "" V2
      NOTIFIERS="$NOTIFIERS
  - type: telegram
    config:
      token: \"$V1\"
      chat_id: \"$V2\"" ;;
    webhook)
      ask "  Webhook URL" "" V1
      ask "  Optional header 'Key: Value' (blank for none)" "" V2
      NOTIFIERS="$NOTIFIERS
  - type: webhook
    config:
      url: \"$V1\""
      if [ -n "$V2" ]; then
        HK=$(printf '%s' "$V2" | sed 's/:.*//' | sed 's/[[:space:]]*$//')
        HV=$(printf '%s' "$V2" | sed 's/^[^:]*:[[:space:]]*//')
        NOTIFIERS="$NOTIFIERS
      headers:
        $HK: \"$HV\""
      fi ;;
    *) warn "unknown type '$NTYPE', skipping" ;;
  esac
}

KEEP_NOTIFIERS=n
[ -n "$CFG_NOTIFIERS" ] && ask_yn "Keep the $CFG_NOTIFIER_COUNT existing notifier(s)?" "y" KEEP_NOTIFIERS
if [ "$KEEP_NOTIFIERS" = "y" ]; then
  # Reuse the previous notifiers block verbatim (leading newline matches the
  # format add_notifier builds, so the writer below emits valid YAML).
  NOTIFIERS=$(printf '\n%s' "$CFG_NOTIFIERS")
else
  info "Add at least one notifier"
  add_notifier
  while : ; do
    ask_yn "Add another notifier?" "n" MORE
    [ "$MORE" = "y" ] || break
    add_notifier
  done
fi
[ -n "$NOTIFIERS" ] || err "no notifiers configured"

# ---------- write config (interval only when the watcher is enabled) ----------
info "Writing config to $CONFIG_PATH"
$SVC mkdir -p "$CONFIG_DIR"
[ -f "$CONFIG_PATH" ] && { $SVC cp "$CONFIG_PATH" "$CONFIG_PATH.bak"; info "Backed up existing config to $CONFIG_PATH.bak"; }
{
  printf 'watch:\n'
  printf '  local:\n    enabled: %s\n' "$(yesbool "$LOCAL_ENABLED")"
  [ "$LOCAL_ENABLED" = "y" ] && printf '    interval: %s\n' "$LOCAL_INTERVAL"
  printf '  public:\n    enabled: %s\n' "$(yesbool "$PUBLIC_ENABLED")"
  [ "$PUBLIC_ENABLED" = "y" ] && printf '    interval: %s\n' "$PUBLIC_INTERVAL"
  printf 'gateway:\n  enabled: %s\n' "$(yesbool "$GATEWAY_ENABLED")"
  [ "$GATEWAY_ENABLED" = "y" ] && printf '  listen: "%s"\n' "$GATEWAY_LISTEN"
  printf 'notifiers:%s\n' "$NOTIFIERS"
} > "$TMP/config.yaml"
$SVC cp "$TMP/config.yaml" "$CONFIG_PATH"
$SVC chmod 0600 "$CONFIG_PATH"

# ---------- install binary + service ----------
info "Installing binary to $BIN_DIR/ipnotify"
$BIN mkdir -p "$BIN_DIR"
if command -v install >/dev/null 2>&1; then
  $BIN install -m 0755 "$SRC_BIN" "$BIN_DIR/ipnotify"
else
  $BIN cp "$SRC_BIN" "$BIN_DIR/ipnotify" && $BIN chmod 0755 "$BIN_DIR/ipnotify"
fi

# Fail fast if the generated config doesn't parse (otherwise the service would
# install but crash-loop and the gateway would never come up).
info "Validating config"
$SVC "$BIN_DIR/ipnotify" validate -c "$CONFIG_PATH" || err "config validation failed; not installing service"

info "Installing and starting service"
# Make re-install idempotent: clear any previous install first (no-op on a
# fresh machine). Otherwise `service install` fails when the LaunchAgent/unit
# already exists ("Init already exists: ...ipnotify.plist").
$SVC "$BIN_DIR/ipnotify" service stop      -c "$CONFIG_PATH" >/dev/null 2>&1 || true
$SVC "$BIN_DIR/ipnotify" service uninstall -c "$CONFIG_PATH" >/dev/null 2>&1 || true
$SVC "$BIN_DIR/ipnotify" service install -c "$CONFIG_PATH" || err "service install failed"
$SVC "$BIN_DIR/ipnotify" service start   -c "$CONFIG_PATH" || err "service start failed"

# ---------- self-test (same privilege as the service so it can read the config) ----------
info "Sending a test notification"
if $SVC "$BIN_DIR/ipnotify" test -c "$CONFIG_PATH"; then
  info "Self-test passed"
else
  warn "self-test reported failures; check your notifier config"
fi

# ---------- summary ----------
MGR="ipnotify service"
[ "$USER_SERVICE" = "1" ] || MGR="sudo ipnotify service"
cat <<EOF

✅ IPNotify installed.

  Config:  $CONFIG_PATH
  Manage:  $MGR status | stop | restart | uninstall
EOF
[ "$GATEWAY_ENABLED" = "y" ] && cat <<EOF
  Gateway: curl http://$GATEWAY_LISTEN/status
           curl -XPOST http://$GATEWAY_LISTEN/test
EOF
