# Makes the Windows distribution (bin\dist.ps1), unzips it to dist\install\, checks the installer
# against SHA256SUMS and runs it without asking: only its progress bar shows, no desktop shortcut, and
# aex opens once it is installed. -Wizard runs the installer's wizard instead. -Arch and -SkipTests go
# to dist.ps1. Waits for the installer and exits with its exit code.
param([ValidateSet('amd64', 'arm64')][string]$Arch = 'amd64', [switch]$SkipTests, [switch]$Wizard)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')

& (Join-Path $PSScriptRoot 'dist.ps1') -Arch $Arch -SkipTests:$SkipTests
if ($LASTEXITCODE) { exit $LASTEXITCODE }

# The zip dist.ps1 just made (its name from ProductVersion in winres\winres.json, as there).
$winres = Get-Content -Raw (Join-Path $root 'winres\winres.json')
if ($winres -notmatch '"ProductVersion":\s*"([^"]+)"') { throw 'No ProductVersion in winres\winres.json' }
$name = "aex-$($Matches[1])-windows-$Arch"
$zip = Join-Path $root "dist\$name.zip"
if (-not (Test-Path $zip)) { throw "No $zip" }

$out = Join-Path $root 'dist\install'
if (Test-Path $out) { Remove-Item -Recurse -Force $out }
Expand-Archive -Path $zip -DestinationPath $out
$setup = Join-Path $out "$name-setup.exe"

# The installer's line in SHA256SUMS.
$want = Get-Content (Join-Path $out 'SHA256SUMS') | ForEach-Object {
    if ($_ -match "^([0-9a-f]{64})  $([regex]::Escape("$name-setup.exe"))$") { $Matches[1] }
} | Select-Object -First 1
if (-not $want) { throw "No $name-setup.exe in SHA256SUMS" }
if ((Get-FileHash -Algorithm SHA256 $setup).Hash.ToLower() -ne $want) { throw "$name-setup.exe does not match SHA256SUMS" }

Write-Host "Running $setup"
$p = if ($Wizard) {
    Start-Process -FilePath $setup -Wait -PassThru
} else {
    Start-Process -FilePath $setup -ArgumentList '/SILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/MERGETASKS="!desktopicon"', '/LAUNCH' -Wait -PassThru
}
if ($p.ExitCode) { Write-Host "Installer exited with $($p.ExitCode)" }
exit $p.ExitCode
