<#
.SYNOPSIS
  Install rill. Run it again to update; it stops when what you have is current.

.EXAMPLE
  irm https://raw.githubusercontent.com/apptivitypl/rill/main/install.ps1 | iex

.EXAMPLE
  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/apptivitypl/rill/main/install.ps1))) -Version v0.1.0
#>
[CmdletBinding()]
param(
  [string]$Version = $env:RILL_VERSION,
  [string]$Dir = $env:RILL_INSTALL_DIR,
  [switch]$Force,
  [switch]$RequireSignature
)

$ErrorActionPreference = 'Stop'
$repo = 'apptivitypl/rill'

function Fail($message) {
  [Console]::Error.WriteLine("install: $message")
  exit 1
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  'AMD64' { 'amd64' }
  'ARM64' { 'arm64' }
  default { Fail "unsupported architecture $($env:PROCESSOR_ARCHITECTURE); see https://github.com/$repo/releases" }
}

if (-not $Dir) { $Dir = Join-Path $env:LOCALAPPDATA 'Programs\rill' }
if ($env:RILL_FORCE) { $Force = $true }
if ($env:RILL_REQUIRE_SIGNATURE) { $RequireSignature = $true }
if (-not [System.IO.Path]::IsPathRooted($Dir)) { Fail "-Dir needs an absolute path, got $Dir" }

if (-not $Version) {
  try {
    $Version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
  } catch {
    Fail "could not find the latest release; pass -Version"
  }
}
$number = $Version.TrimStart('v')

$target = Join-Path $Dir 'rill.exe'
if (-not $Force -and (Test-Path $target)) {
  $current = (& $target version 2>$null | Select-Object -First 1)
  if ($current -match [regex]::Escape($number)) {
    Write-Host "rill $Version is already installed at $target"
    exit 0
  }
}

$archive = "rill_${number}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$Version"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("rill-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
  Write-Host "downloading rill $Version for windows/$arch"
  try {
    Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
  } catch {
    Fail "no archive for windows/$arch in $Version"
  }
  Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

  $got = (Get-FileHash (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLower()
  $line = Get-Content (Join-Path $tmp 'checksums.txt') |
    Where-Object { $_ -match "(^|\s)\*?$([regex]::Escape($archive))\s*$" } | Select-Object -First 1
  if (-not $line) { Fail "$archive is not listed in checksums.txt" }
  $want = ($line -split '\s+')[0].ToLower()
  if ($got -ne $want) { Fail "checksum mismatch for $archive; refusing to install" }
  Write-Host 'checksum ok'

  if (Get-Command cosign -ErrorAction SilentlyContinue) {
    $signed = $true
    try {
      Invoke-WebRequest "$base/checksums.txt.pem" -OutFile (Join-Path $tmp 'checksums.txt.pem') -UseBasicParsing
      Invoke-WebRequest "$base/checksums.txt.sig" -OutFile (Join-Path $tmp 'checksums.txt.sig') -UseBasicParsing
    } catch {
      $signed = $false
    }
    if (-not $signed -and $RequireSignature) {
      Fail "no signature published for $Version and -RequireSignature was given"
    }
    if ($signed) {
      & cosign verify-blob (Join-Path $tmp 'checksums.txt') `
        --certificate (Join-Path $tmp 'checksums.txt.pem') `
        --signature (Join-Path $tmp 'checksums.txt.sig') `
        --certificate-identity-regexp "https://github\.com/$repo/\.github/workflows/release\.yml@.*" `
        --certificate-oidc-issuer https://token.actions.githubusercontent.com 2>$null | Out-Null
      if ($LASTEXITCODE -ne 0) { Fail 'the signature on checksums.txt did not verify' }
      Write-Host 'signature ok'
    } else {
      Write-Host 'no signature published for this release'
    }
  } elseif ($RequireSignature) {
    Fail 'cosign is not installed and -RequireSignature was given'
  } else {
    Write-Host 'cosign is not installed, so the signature was not checked'
  }

  Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force
  New-Item -ItemType Directory -Path $Dir -Force | Out-Null
  Move-Item -Path (Join-Path $tmp 'rill.exe') -Destination $target -Force
  Write-Host "installed rill $Version to $target"

  $user = [Environment]::GetEnvironmentVariable('Path', 'User')
  if ($user -notlike "*$Dir*") {
    [Environment]::SetEnvironmentVariable('Path', "$user;$Dir", 'User')
    Write-Host ''
    Write-Host "$Dir was added to your PATH. Open a new terminal for it to take effect."
  }
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
