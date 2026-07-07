#!/bin/sh
# IPNotify installer — download the matching prebuilt binary from the latest
# GitHub release and install it as a background service.
#
# Quick start (Linux / macOS / OpenWrt):
#   curl -fsSL https://raw.githubusercontent.com/mortimerzhu/IPNotify/main/install.sh | sh
#   wget -qO- https://raw.githubusercontent.com/mortimerzhu/IPNotify/main/install.sh | sh
#
# (Developing the project? Use scripts/install.sh to build from source instead.)
#
# Options (environment variables):
#   IPNOTIFY_VERSION=v0.0.1   install a specific release instead of the latest
#   IPNOTIFY_REPO=owner/name  pull from a fork
#   IPNOTIFY_NO_INSTALL=1     download + verify + extract only (drops ./ipnotify), skip install
#   Any scripts/install.sh variable (ASSUME_YES, LOCAL_ENABLED, GATEWAY_LISTEN, ...)
#   is passed straight through to the bundled installer.
set -eu

REPO="${IPNOTIFY_REPO:-mortimerzhu/IPNotify}"

err()  { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '\033[1;34m==>\033[0m %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

fetch_file()   { # url outfile
  if   have curl; then curl -fsSL "$1" -o "$2"
  elif have wget; then wget -qO "$2" "$1"
  else err "need curl or wget"; fi
}
fetch_stdout() { # url
  if   have curl; then curl -fsSL "$1"
  elif have wget; then wget -qO - "$1"
  else err "need curl or wget"; fi
}

# ---------- detect OS / arch (must match GoReleaser asset names) ----------
OS=$(uname -s)
case "$OS" in
  Linux)  OS=linux ;;
  Darwin) OS=darwin ;;
  *) err "unsupported OS '$OS' — on Windows use install.ps1" ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)            ARCH=amd64 ;;
  aarch64|arm64)           ARCH=arm64 ;;
  armv7l|armv6l|armhf|arm) ARCH=armv7 ;;
  *) err "no prebuilt binary for arch '$ARCH' (built: amd64, arm64, armv7).
Build from source instead: git clone the repo and run scripts/install.sh" ;;
esac
info "Detected platform: $OS/$ARCH"

# ---------- resolve the release tag ----------
if [ -n "${IPNOTIFY_VERSION:-}" ]; then
  TAG="$IPNOTIFY_VERSION"
elif have curl; then
  # Follow the /releases/latest redirect — no API call, so no rate limit.
  TAG=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        "https://github.com/$REPO/releases/latest" | sed 's#.*/tag/##')
else
  TAG=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
fi
[ -n "$TAG" ] || err "could not determine the release tag"
VER="${TAG#v}"   # asset filenames drop the leading "v"
info "Release: $TAG"

ASSET="ipnotify_${VER}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$TAG"

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/ipnotify-dl.$$"); mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT

info "Downloading $ASSET"
fetch_file "$BASE/$ASSET" "$TMP/$ASSET" \
  || err "download failed — $ASSET may not exist for $TAG"

# ---------- verify checksum (best effort) ----------
if fetch_file "$BASE/checksums.txt" "$TMP/checksums.txt" 2>/dev/null; then
  WANT=$(grep " $ASSET\$" "$TMP/checksums.txt" 2>/dev/null | awk '{print $1}')
  if [ -n "${WANT:-}" ]; then
    if   have sha256sum; then GOT=$(sha256sum "$TMP/$ASSET" | awk '{print $1}')
    elif have shasum;    then GOT=$(shasum -a 256 "$TMP/$ASSET" | awk '{print $1}')
    else GOT=""; info "no sha256 tool found; skipping checksum verification"; fi
    if [ -n "$GOT" ]; then
      [ "$GOT" = "$WANT" ] || err "checksum mismatch for $ASSET"
      info "Checksum verified"
    fi
  fi
else
  info "checksums.txt unavailable; skipping checksum verification"
fi

# ---------- extract ----------
tar -xzf "$TMP/$ASSET" -C "$TMP" || err "failed to extract $ASSET"
BIN="$TMP/ipnotify"
[ -f "$BIN" ] || err "ipnotify binary missing from archive"
chmod +x "$BIN" 2>/dev/null || true
# Clear the macOS quarantine flag so launchd can exec the binary without a prompt.
if [ "$OS" = "darwin" ] && have xattr; then xattr -d com.apple.quarantine "$BIN" 2>/dev/null || true; fi

# ---------- download-only mode ----------
if [ "${IPNOTIFY_NO_INSTALL:-0}" = "1" ]; then
  OUT="$PWD/ipnotify"
  cp "$BIN" "$OUT"; chmod +x "$OUT" 2>/dev/null || true
  info "Saved binary to $OUT (install skipped)"
  exit 0
fi

# ---------- hand off to the bundled interactive installer ----------
INSTALLER="$TMP/scripts/install.sh"
[ -f "$INSTALLER" ] || err "bundled installer (scripts/install.sh) missing from archive"
info "Launching installer"
IPNOTIFY_BINARY="$BIN" sh "$INSTALLER"
