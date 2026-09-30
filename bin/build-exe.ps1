# Builds the release exe (default dist\aex.exe): .env built in, reads .env next to the exe.
# -Out <file> builds elsewhere, e.g. while dist\aex.exe is running.
param([string]$Out)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
if (-not $Out) { $Out = Join-Path $root 'dist\aex.exe' }
elseif (-not [IO.Path]::IsPathRooted($Out)) { $Out = [IO.Path]::GetFullPath((Join-Path (Get-Location) $Out)) }

# A running exe cannot be overwritten but can be renamed, so an open window does not block the build.
foreach ($f in @($Out) + @(Get-ChildItem (Join-Path (Split-Path $Out) 'plugins\*.exe') -ErrorAction SilentlyContinue | ForEach-Object FullName)) {
    Get-ChildItem "$f.old*" -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue
    if (Test-Path $f) { Move-Item $f "$f.old$([DateTime]::Now.Ticks)" }
}

# Plugins first: their hashes are built into aex as pre-approved (see bin\_plugin-hashes.ps1).
$plugins = New-Item -ItemType Directory -Force (Join-Path (Split-Path $Out) 'plugins')
go build -C $root -trimpath -ldflags '-s -w' -o $plugins.FullName ./plugins/...
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$hashes = & (Join-Path $PSScriptRoot '_plugin-hashes.ps1') $plugins.FullName

# -trimpath also makes it a release build: it stops looking for the source checkout.
# desktop,production: the Wails build tags the window needs. builtAt: when, for Settings > About.
$builtAt = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
go build -C $root -tags desktop,production -trimpath -ldflags "-s -w -X aex/internal/plugin.preApproved=$hashes -X aex/internal/about.builtAt=$builtAt" -o $Out .
if ($LASTEXITCODE) { exit $LASTEXITCODE }
Write-Host "Built $Out"
Get-ChildItem $plugins -Filter *.exe | ForEach-Object { Write-Host "Built $($_.FullName)" }
