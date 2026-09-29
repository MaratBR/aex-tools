# Shared launcher: builds aex from source (dev build, reads .env from the repo) and
# runs tool <Name>, forwarding args and exit code. Name "aex" opens the menu. Plugins (plugins\<name>)
# are built into dist\dev\plugins.
param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest
)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$exe = Join-Path $root 'dist\dev\aex.exe'

# A running exe cannot be overwritten but can be renamed, so an open menu does not block the build.
Get-ChildItem "$exe.old*" -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue
if (Test-Path $exe) { Move-Item $exe "$exe.old$([DateTime]::Now.Ticks)" }
go build -C $root -o $exe .
if ($LASTEXITCODE) { exit $LASTEXITCODE }
$plugins = New-Item -ItemType Directory -Force (Join-Path (Split-Path $exe) 'plugins')
go build -C $root -o $plugins.FullName ./plugins/...
if ($LASTEXITCODE) { exit $LASTEXITCODE }

if ($Name -eq 'aex') { & $exe @Rest } else { & $exe $Name @Rest }
exit $LASTEXITCODE
