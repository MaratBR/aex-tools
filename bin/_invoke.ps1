# Shared launcher: builds aex-cli.exe from source (dev build, reads .env from the repo) and
# runs tool <Name>, forwarding args and exit code. Name "aex" opens the window. Plugins (plugins\<name>)
# are built into dist\dev\plugins first, and their hashes built into aex as pre-approved.
param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest
)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$exe = Join-Path $root 'dist\dev\aex-cli.exe'

# A running exe cannot be overwritten but can be renamed, so an open window does not block the build.
Get-ChildItem "$exe.old*" -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue
if (Test-Path $exe) { Move-Item $exe "$exe.old$([DateTime]::Now.Ticks)" }
$plugins = New-Item -ItemType Directory -Force (Join-Path (Split-Path $exe) 'plugins')
go build -C $root -o $plugins.FullName ./plugins/...
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$hashes = & (Join-Path $PSScriptRoot '_plugin-hashes.ps1') $plugins.FullName
go build -C $root -tags desktop,production -ldflags "-X aex/internal/plugin.preApproved=$hashes" -o $exe .
if ($LASTEXITCODE) { exit $LASTEXITCODE }

if ($Name -eq 'aex') { & $exe @Rest } else { & $exe $Name @Rest }
exit $LASTEXITCODE
