<#
.SYNOPSIS
  Remove gopage and, if you ask, everything it has cached.

.EXAMPLE
  irm https://raw.githubusercontent.com/apptivitypl/gopage/main/uninstall.ps1 | iex

.EXAMPLE
  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/apptivitypl/gopage/main/uninstall.ps1))) -Purge
#>
[CmdletBinding()]
param(
  [string]$Dir = $env:GOPAGE_INSTALL_DIR,
  [switch]$Purge,
  [switch]$Yes
)

$ErrorActionPreference = 'Stop'

function Fail($message) { Write-Error "uninstall: $message"; exit 1 }

if ($Dir) {
  $target = Join-Path $Dir 'gopage.exe'
  if (-not (Test-Path $target)) { Fail "no gopage.exe in $Dir" }
} else {
  $found = Get-Command gopage -ErrorAction SilentlyContinue
  if (-not $found) {
    Write-Host 'gopage is not on your PATH; nothing to remove'
    exit 0
  }
  $target = $found.Source
  $Dir = Split-Path -Parent $target
}

$version = (& $target version 2>$null | Select-Object -First 1)
if (-not $version) { $version = 'gopage' }
Write-Host "found $version at $target"

$cache = Join-Path $env:LOCALAPPDATA 'gopage'

if (-not $Yes) {
  Write-Host ''
  Write-Host 'about to remove:'
  Write-Host "  $target"
  if ($Purge -and (Test-Path $cache)) { Write-Host "  $cache" }
  $answer = Read-Host 'continue? [y/N]'
  if ($answer -notmatch '^(y|yes)$') {
    Write-Host 'nothing was removed'
    exit 0
  }
}

Remove-Item -Force $target
Write-Host "removed $target"

if ($Purge) {
  if (Test-Path $cache) {
    Remove-Item -Recurse -Force $cache
    Write-Host "removed $cache"
  } else {
    Write-Host "no cache at $cache"
  }
}

$user = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($user -like "*$Dir*" -and -not (Get-ChildItem $Dir -ErrorAction SilentlyContinue)) {
  $kept = ($user -split ';' | Where-Object { $_ -and $_ -ne $Dir }) -join ';'
  [Environment]::SetEnvironmentVariable('Path', $kept, 'User')
  Write-Host "removed $Dir from your PATH"
}

Write-Host ''
Write-Host 'projects you created were left alone. To remove one, delete its directory.'
