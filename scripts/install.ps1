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
Write-Host "==> Configure watchers" -ForegroundColor Cyan
$localEnabled = AskYN "Enable local (LAN) IP watcher?" "y"
if ($localEnabled -eq "y") { $localInterval = Ask "  local poll interval (seconds)" "10" }
$publicEnabled = AskYN "Enable public (egress) IP watcher?" "y"
if ($publicEnabled -eq "y") { $publicInterval = Ask "  public poll interval (seconds)" "60" }
if ($localEnabled -ne "y" -and $publicEnabled -ne "y") { throw "at least one watcher must be enabled" }

Write-Host "==> Configure gateway" -ForegroundColor Cyan
$gatewayEnabled = AskYN "Enable gateway?" "y"
if ($gatewayEnabled -eq "y") { $gatewayListen = Ask "  gateway listen address" "127.0.0.1:8555" }

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

Write-Host "==> Add at least one notifier" -ForegroundColor Cyan
Add-Notifier
while ((AskYN "Add another notifier?" "n") -eq "y") { Add-Notifier }
if (-not $notifiers) { throw "no notifiers configured" }

# --- write config ---
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
if (Test-Path $ConfigPath) { Copy-Item -Force $ConfigPath "$ConfigPath.bak" }
$lines = @("watch:", "  local:", "    enabled: $(YesBool $localEnabled)")
if ($localEnabled -eq "y") { $lines += "    interval: $localInterval" }
$lines += @("  public:", "    enabled: $(YesBool $publicEnabled)")
if ($publicEnabled -eq "y") { $lines += "    interval: $publicInterval" }
$lines += @("gateway:", "  enabled: $(YesBool $gatewayEnabled)")
if ($gatewayEnabled -eq "y") { $lines += "  listen: `"$gatewayListen`"" }
$lines += "notifiers:$notifiers"
Set-Content -Path $ConfigPath -Value ($lines -join "`n") -Encoding UTF8
Write-Host "==> Wrote $ConfigPath" -ForegroundColor Cyan

# --- validate config before installing (avoid a crash-looping service) ---
& $BinPath validate -c $ConfigPath
if ($LASTEXITCODE -ne 0) { throw "config validation failed; not installing service" }

# --- install + start service ---
& $BinPath service install -c $ConfigPath
& $BinPath service start -c $ConfigPath

Write-Host "==> Self-test" -ForegroundColor Cyan
& $BinPath test -c $ConfigPath

Write-Host ""
Write-Host "IPNotify installed." -ForegroundColor Green
Write-Host "  Manage: ipnotify service status | stop | restart | uninstall"
if ($gatewayEnabled -eq "y") { Write-Host "  Gateway: curl http://$gatewayListen/status" }
