# IPNotify installer for Windows — download the matching prebuilt binary from the
# latest GitHub release and install it as a Windows service.
#
# Run in an ELEVATED PowerShell (service install needs admin):
#   irm https://raw.githubusercontent.com/mortimerzhu/IPNotify/main/install.ps1 | iex
#
# (Developing the project? Use scripts\install.ps1 to build from source instead.)
#
# Options (environment variables):
#   $env:IPNOTIFY_VERSION = "v0.0.1"        install a specific release
#   $env:IPNOTIFY_REPO    = "owner/name"    pull from a fork
#   $env:IPNOTIFY_NO_INSTALL = "1"          download + verify + extract only

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = if ($env:IPNOTIFY_REPO) { $env:IPNOTIFY_REPO } else { "mortimerzhu/IPNotify" }

# --- detect arch (must match GoReleaser asset names) ---
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { "amd64" }
  "ARM64" { "arm64" }
  default { throw "no prebuilt binary for arch '$($env:PROCESSOR_ARCHITECTURE)' (built: amd64, arm64)" }
}
Write-Host "==> Detected platform: windows/$arch" -ForegroundColor Cyan

# --- resolve the release tag ---
if ($env:IPNOTIFY_VERSION) {
  $tag = $env:IPNOTIFY_VERSION
} else {
  # Resolve "latest" via the github.com /releases/latest redirect
  # (302 -> /releases/tag/vX.Y.Z). Unlike api.github.com this is NOT subject to
  # the unauthenticated 60-requests/hour rate limit.
  try {
    $req = [System.Net.HttpWebRequest]::Create("https://github.com/$Repo/releases/latest")
    $req.AllowAutoRedirect = $false
    $req.UserAgent = "ipnotify-installer"
    $resp = $req.GetResponse()
    $loc = $resp.Headers["Location"]
    $resp.Close()
    if ($loc) { $tag = ($loc -split "/")[-1] }
  } catch { $tag = $null }
  # Fallback to the API (may hit the rate limit) if the redirect yielded nothing.
  if (-not $tag) {
    $tag = (Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest").tag_name
  }
}
if (-not $tag) { throw "could not determine the release tag" }
$ver = $tag.TrimStart("v")   # asset filenames drop the leading "v"
Write-Host "==> Release: $tag" -ForegroundColor Cyan

$asset = "ipnotify_${ver}_windows_${arch}.zip"
$base  = "https://github.com/$Repo/releases/download/$tag"

$tmp = Join-Path $env:TEMP ("ipnotify-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
  Write-Host "==> Downloading $asset" -ForegroundColor Cyan
  Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $tmp $asset)

  # --- verify checksum (best effort) ---
  try {
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")
    $line = Select-String -Path (Join-Path $tmp "checksums.txt") -SimpleMatch $asset | Select-Object -First 1
    if ($line) {
      $want = ($line.Line -split "\s+")[0].ToLower()
      $got  = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLower()
      if ($want -ne $got) { throw "checksum mismatch for $asset" }
      Write-Host "==> Checksum verified" -ForegroundColor Cyan
    }
  } catch { Write-Host "==> checksum verification skipped ($_)" -ForegroundColor Yellow }

  # --- extract ---
  Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath $tmp -Force
  $bin = Join-Path $tmp "ipnotify.exe"
  if (-not (Test-Path $bin)) { throw "ipnotify.exe missing from archive" }

  if ($env:IPNOTIFY_NO_INSTALL -eq "1") {
    $out = Join-Path (Get-Location) "ipnotify.exe"
    Copy-Item -Force $bin $out
    Write-Host "==> Saved binary to $out (install skipped)" -ForegroundColor Green
    return
  }

  # --- hand off to the bundled installer ---
  $installer = Join-Path $tmp "scripts\install.ps1"
  if (-not (Test-Path $installer)) { throw "bundled installer (scripts\install.ps1) missing from archive" }
  Write-Host "==> Launching installer" -ForegroundColor Cyan
  $env:IPNOTIFY_BINARY = $bin
  & $installer
}
finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
