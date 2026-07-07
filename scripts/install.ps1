# IPNotify interactive installer for Windows (registers a Windows service via SCM).
# Run in an elevated PowerShell:  powershell -ExecutionPolicy Bypass -File scripts\install.ps1
# Skip build with a prebuilt binary:  $env:IPNOTIFY_BINARY="C:\path\ipnotify.exe"; .\scripts\install.ps1

$ErrorActionPreference = "Stop"

function Ask($prompt, $default) {
  if ($default) { $p = "$prompt [$default]" } else { $p = $prompt }
  $a = Read-Host $p
  if ([string]::IsNullOrWhiteSpace($a)) { return $default } else { return $a }
}
function AskYN($prompt, $default) { return (Ask "$prompt (y/n)" $default) }
function YesBool($v) { if ($v -eq "y") { "true" } else { "false" } }

# Map a YAML bool ("true"/"false") to the y/n prompt default, falling back to
# $fallback for anything else (missing key, unexpected value).
function BoolYN($v, $fallback) {
  if ($v -eq "true") { "y" } elseif ($v -eq "false") { "n" } else { $fallback }
}
# Return $v (quotes stripped) when set, otherwise $fallback.
function Def($v, $fallback) {
  if ($v) { $v.Trim('"').Trim("'") } else { $fallback }
}

# Best-effort parser for a config.yaml previously written by this installer (or
# a compatible hand-edited one). Returns the scalar watcher/gateway settings and
# the raw notifiers block so a re-install can offer the current values as
# defaults instead of asking for everything again. Deliberately minimal — it
# only understands the small, fixed shape this script emits.
function Read-ExistingConfig($path) {
  $r = @{
    LocalEnabled=$null; LocalInterval=$null; PublicEnabled=$null; PublicInterval=$null
    PublicRegion=$null; GatewayEnabled=$null; GatewayListen=$null; Notifiers=$null; NotifierCount=0
  }
  if (-not (Test-Path $path)) { return $r }

  $top = ""; $sub = ""; $inNotifiers = $false; $notifierLines = @()
  foreach ($line in (Get-Content -Path $path -Encoding UTF8)) {
    if ($line -match '^\s*#') { if ($inNotifiers) { $notifierLines += $line }; continue }
    if ($line.TrimEnd() -eq "") { if ($inNotifiers) { $notifierLines += $line }; continue }

    if ($line -match '^\S') {                       # top-level key (column 0)
      $inNotifiers = $false; $sub = ""
      if     ($line -match '^watch:')     { $top = "watch" }
      elseif ($line -match '^gateway:')   { $top = "gateway" }
      elseif ($line -match '^notifiers:') { $top = "notifiers"; $inNotifiers = $true }
      else                                { $top = "" }
      continue
    }
    if ($inNotifiers) { $notifierLines += $line; continue }

    $indent = ($line -replace '^(\s*).*', '$1').Length
    $t = $line.Trim()
    if ($top -eq "watch") {
      if ($indent -eq 2 -and $t -match '^local:')  { $sub = "local";  continue }
      if ($indent -eq 2 -and $t -match '^public:') { $sub = "public"; continue }
      if ($indent -ge 4 -and $t -match '^enabled:\s*(\S+)') {
        if ($sub -eq "local") { $r.LocalEnabled = $matches[1] } elseif ($sub -eq "public") { $r.PublicEnabled = $matches[1] }
      }
      if ($indent -ge 4 -and $t -match '^interval:\s*(\S+)') {
        if ($sub -eq "local") { $r.LocalInterval = $matches[1] } elseif ($sub -eq "public") { $r.PublicInterval = $matches[1] }
      }
      if ($indent -ge 4 -and $sub -eq "public" -and $t -match '^region:\s*(\S+)') { $r.PublicRegion = $matches[1] }
    } elseif ($top -eq "gateway") {
      if ($t -match '^enabled:\s*(\S+)') { $r.GatewayEnabled = $matches[1] }
      if ($t -match '^listen:\s*(\S+)')  { $r.GatewayListen  = $matches[1] }
    }
  }

  # Drop trailing blank lines via a cursor (avoids the PowerShell `0..-1`
  # range pitfall when the array shrinks to a single blank element).
  $end = $notifierLines.Count - 1
  while ($end -ge 0 -and $notifierLines[$end].Trim() -eq "") { $end-- }
  if ($end -ge 0) {
    $notifierLines = $notifierLines[0..$end]
    $r.Notifiers = ($notifierLines -join "`n")
    $r.NotifierCount = ($notifierLines | Where-Object { $_ -match '^\s*-\s*type:' }).Count
  }
  return $r
}

# Require admin (service install needs it)
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()
          ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Please run this script in an elevated (Administrator) PowerShell." }

$BinDir = Join-Path $env:ProgramFiles "IPNotify"
$ConfigDir = Join-Path $env:ProgramData "IPNotify"
$ConfigPath = Join-Path $ConfigDir "config.yaml"
$BinPath = Join-Path $BinDir "ipnotify.exe"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir

Write-Host "==> Target: $BinPath  config: $ConfigPath" -ForegroundColor Cyan

# --- obtain binary ---
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
if ($env:IPNOTIFY_BINARY) {
  Copy-Item -Force $env:IPNOTIFY_BINARY $BinPath
  Write-Host "==> Using prebuilt binary" -ForegroundColor Cyan
} elseif (Get-Command go -ErrorAction SilentlyContinue) {
  Write-Host "==> Building from source..." -ForegroundColor Cyan
  Push-Location $RepoRoot
  try { & go build -o $BinPath ./cmd/ipnotify } finally { Pop-Location }
} else {
  throw "Go toolchain not found and IPNOTIFY_BINARY not set. Install Go or set `$env:IPNOTIFY_BINARY."
}

# --- interactive config ---
# Seed prompt defaults from an existing config so a re-install can just accept
# the previous answers (press Enter through) instead of retyping everything.
$cfg = Read-ExistingConfig $ConfigPath
if ($cfg.LocalEnabled -or $cfg.PublicEnabled -or $cfg.GatewayListen -or $cfg.Notifiers) {
  Write-Host "==> Found existing config — using its values as defaults (press Enter to keep)" -ForegroundColor Cyan
}

Write-Host "==> Configure watchers" -ForegroundColor Cyan
$localEnabled = AskYN "Enable local (LAN) IP watcher?" (BoolYN $cfg.LocalEnabled "y")
if ($localEnabled -eq "y") { $localInterval = Ask "  local poll interval (seconds)" (Def $cfg.LocalInterval "10") }
$publicEnabled = AskYN "Enable WAN (public egress) IP watcher?" (BoolYN $cfg.PublicEnabled "y")
if ($publicEnabled -eq "y") {
  $publicInterval = Ask "  WAN poll interval (seconds)" (Def $cfg.PublicInterval "60")
  # Region picks the default IP-echo source list; "cn" uses domestic services
  # so a transparent proxy that direct-routes China still yields the real IP.
  $publicRegion = Ask "  WAN IP source region (auto/cn/global)" (Def $cfg.PublicRegion "auto")
}
if ($localEnabled -ne "y" -and $publicEnabled -ne "y") { throw "at least one watcher must be enabled" }

Write-Host "==> Configure gateway" -ForegroundColor Cyan
$gatewayEnabled = AskYN "Enable gateway?" (BoolYN $cfg.GatewayEnabled "y")
if ($gatewayEnabled -eq "y") { $gatewayListen = Ask "  gateway listen address" (Def $cfg.GatewayListen "127.0.0.1:8555") }

$notifiers = ""
function Add-Notifier {
  $t = Ask "Notifier type (dingtalk/feishu/telegram/webhook)" ""
  switch ($t) {
    "dingtalk" {
      $u = Ask "  DingTalk webhook URL" ""; $s = Ask "  DingTalk secret (blank to skip)" ""
      $script:notifiers += "`n  - type: dingtalk`n    config:`n      webhook: `"$u`""
      if ($s) { $script:notifiers += "`n      secret: `"$s`"" }
    }
    "feishu" {
      $u = Ask "  Feishu webhook URL" ""; $s = Ask "  Feishu secret (blank to skip)" ""
      $script:notifiers += "`n  - type: feishu`n    config:`n      webhook: `"$u`""
      if ($s) { $script:notifiers += "`n      secret: `"$s`"" }
    }
    "telegram" {
      $tok = Ask "  Telegram bot token" ""; $c = Ask "  Telegram chat_id" ""
      $script:notifiers += "`n  - type: telegram`n    config:`n      token: `"$tok`"`n      chat_id: `"$c`""
    }
    "webhook" {
      $u = Ask "  Webhook URL" ""
      $script:notifiers += "`n  - type: webhook`n    config:`n      url: `"$u`""
    }
    default { Write-Host "unknown type, skipping" -ForegroundColor Yellow }
  }
}

Write-Host "==> Notifiers" -ForegroundColor Cyan
if ($cfg.Notifiers -and (AskYN "Keep the $($cfg.NotifierCount) existing notifier(s)?" "y") -eq "y") {
  # Reuse the previous notifiers block verbatim (keeps webhook URLs / secrets /
  # tokens without re-typing them).
  $notifiers = "`n" + $cfg.Notifiers
} else {
  Write-Host "==> Add at least one notifier" -ForegroundColor Cyan
  Add-Notifier
  while ((AskYN "Add another notifier?" "n") -eq "y") { Add-Notifier }
}
if (-not $notifiers) { throw "no notifiers configured" }

# --- write config ---
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
if (Test-Path $ConfigPath) { Copy-Item -Force $ConfigPath "$ConfigPath.bak" }
$lines = @("watch:", "  local:", "    enabled: $(YesBool $localEnabled)")
if ($localEnabled -eq "y") { $lines += "    interval: $localInterval" }
$lines += @("  public:", "    enabled: $(YesBool $publicEnabled)")
if ($publicEnabled -eq "y") { $lines += "    interval: $publicInterval" }
if ($publicEnabled -eq "y" -and $publicRegion) { $lines += "    region: $publicRegion" }
$lines += @("gateway:", "  enabled: $(YesBool $gatewayEnabled)")
if ($gatewayEnabled -eq "y") { $lines += "  listen: `"$gatewayListen`"" }
$lines += "notifiers:$notifiers"
Set-Content -Path $ConfigPath -Value ($lines -join "`n") -Encoding UTF8
Write-Host "==> Wrote $ConfigPath" -ForegroundColor Cyan

# --- validate config before installing (avoid a crash-looping service) ---
& $BinPath validate -c $ConfigPath
if ($LASTEXITCODE -ne 0) { throw "config validation failed; not installing service" }

# --- install + start service ---
# Make re-install idempotent: clear any previous install first (no-op on a
# fresh machine) so `service install` doesn't fail when the service already
# exists.
& $BinPath service stop -c $ConfigPath 2>$null
& $BinPath service uninstall -c $ConfigPath 2>$null
& $BinPath service install -c $ConfigPath
if ($LASTEXITCODE -ne 0) { throw "service install failed" }
& $BinPath service start -c $ConfigPath
if ($LASTEXITCODE -ne 0) { throw "service start failed" }

Write-Host "==> Self-test" -ForegroundColor Cyan
& $BinPath test -c $ConfigPath

Write-Host ""
Write-Host "IPNotify installed." -ForegroundColor Green
Write-Host "  Manage: ipnotify service status | stop | restart | uninstall"
if ($gatewayEnabled -eq "y") {
  Write-Host "  Test:"
  Write-Host "    Status : curl.exe http://$gatewayListen/status"
  Write-Host "    Notify : curl.exe -X POST http://$gatewayListen/test"
  Write-Host "  (PowerShell: use curl.exe, not curl -- bare 'curl' is an alias for Invoke-WebRequest)"
}
