# Prints the SHA-256 of the plugins built from this repo (plugins\<name>) into folder $Dir, comma
# separated: what the build passes to aex as pre-approved (-X aex/internal/plugin.preApproved).
# Other files in $Dir are left out, so only this repo's plugins run without approval.
param([Parameter(Mandatory = $true)][string]$Dir)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$ext = if ($env:GOOS -eq 'windows' -or (-not $env:GOOS -and $env:OS -eq 'Windows_NT')) { '.exe' } else { '' }
$hashes = foreach ($src in Get-ChildItem (Join-Path $root 'plugins') -Directory) {
    $exe = Join-Path $Dir ($src.Name + $ext)
    if (Test-Path $exe) { (Get-FileHash -Algorithm SHA256 $exe).Hash.ToLower() }
}
$hashes -join ','
