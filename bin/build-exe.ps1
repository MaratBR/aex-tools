# Builds the release exe (default dist\aex.exe): .env built in, reads .env next to the exe.
# -Out <file> builds elsewhere, e.g. while dist\aex.exe is running.
param([string]$Out)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
if (-not $Out) { $Out = Join-Path $root 'dist\aex.exe' }
elseif (-not [IO.Path]::IsPathRooted($Out)) { $Out = [IO.Path]::GetFullPath((Join-Path (Get-Location) $Out)) }

# -trimpath also makes it a release build: it stops looking for the source checkout.
go build -C $root -trimpath -ldflags '-s -w' -o $Out .
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$plugins = New-Item -ItemType Directory -Force (Join-Path (Split-Path $Out) 'plugins')
go build -C $root -trimpath -ldflags '-s -w' -o $plugins.FullName ./plugins/...
if ($LASTEXITCODE) { exit $LASTEXITCODE }
Write-Host "Built $Out"
Get-ChildItem $plugins -Filter *.exe | ForEach-Object { Write-Host "Built $($_.FullName)" }
