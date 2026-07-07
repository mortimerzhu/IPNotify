#!/bin/sh
# IPNotify uninstaller for Linux/macOS/OpenWrt.
#   ./scripts/uninstall.sh            # remove service, keep binary+config
#   PURGE=1 ./scripts/uninstall.sh    # also remove binary and config
set -eu

OS=$(uname -s 2>/dev/null || echo unknown)
IS_OPENWRT=0; [ -f /etc/openwrt_release ] && IS_OPENWRT=1
case "$OS" in
  Darwin)
    USER_SERVICE=1; BIN=/usr/local/bin/ipnotify
    CONFIG_DIR="$HOME/Library/Application Support/IPNotify" ;;
  Linux)
    USER_SERVICE=0
    [ "$IS_OPENWRT" = "1" ] && BIN=/usr/bin/ipnotify || BIN=/usr/local/bin/ipnotify
    CONFIG_DIR=/etc/ipnotify ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

# Service control privilege: none for user service, else root/sudo.
if [ "$USER_SERVICE" = "1" ] || [ "$(id -u)" = "0" ]; then SVC=""; else SVC="sudo"; fi

if [ -x "$BIN" ]; then
  $SVC "$BIN" service stop 2>/dev/null || true
  $SVC "$BIN" service uninstall 2>/dev/null || true
  echo "service removed"
fi

if [ "${PURGE:-0}" = "1" ]; then
  $SVC rm -f "$BIN"
  $SVC rm -rf "$CONFIG_DIR"
  echo "binary and config purged"
else
  echo "kept binary ($BIN) and config ($CONFIG_DIR); set PURGE=1 to remove"
fi
