# Shared launcher: runs scripts/<Name>.js with node, forwarding args and exit code.
param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest
)
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot "..\scripts\$Name.js"
if (-not (Test-Path $script)) { throw "Script not found: $script" }
& node $script @Rest
exit $LASTEXITCODE
