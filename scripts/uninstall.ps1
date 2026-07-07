# IPNotify uninstaller for Windows.
#   powershell -ExecutionPolicy Bypass -File scripts\uninstall.ps1
#   $env:PURGE="1"; .\scripts\uninstall.ps1   # also remove binary + config
$ErrorActionPreference = "SilentlyContinue"

$BinPath = Join-Path (Join-Path $env:ProgramFiles "IPNotify") "ipnotify.exe"
$ConfigDir = Join-Path $env:ProgramData "IPNotify"

if (Test-Path $BinPath) {
  & $BinPath service stop
  & $BinPath service uninstall
  Write-Host "service removed"
}

if ($env:PURGE -eq "1") {
  Remove-Item -Force -Recurse (Split-Path -Parent $BinPath)
  Remove-Item -Force -Recurse $ConfigDir
  Write-Host "binary and config purged"
} else {
  Write-Host "kept binary and config; set `$env:PURGE=1 to remove"
}
