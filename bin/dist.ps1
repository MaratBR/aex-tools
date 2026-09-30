# Makes the Windows distribution: vets and tests, builds the release exe and plugins (bin\build-exe.ps1),
# then zips them with LICENSE and NOTICE to dist\aex-<version>-windows-<arch>.zip.
# -Arch amd64 (default) or arm64. -SkipTests skips go vet and go test.
param([ValidateSet('amd64', 'arm64')][string]$Arch = 'amd64', [switch]$SkipTests)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')

if (-not $SkipTests) {
    Write-Host 'go vet ./...'
    go -C $root vet ./...
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
    Write-Host 'go test ./...'
    go -C $root test ./...
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
}

# The version Settings > About shows (ProductVersion in winres\winres.json).
$winres = Get-Content -Raw (Join-Path $root 'winres\winres.json')
if ($winres -notmatch '"ProductVersion":\s*"([^"]+)"') { throw 'No ProductVersion in winres\winres.json' }
$version = $Matches[1]
$name = "aex-$version-windows-$Arch"
$dist = Join-Path $root 'dist'
$stage = Join-Path $dist $name
$zip = Join-Path $dist "$name.zip"

if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force $stage | Out-Null

$oldGOOS, $oldGOARCH = $env:GOOS, $env:GOARCH
$env:GOOS, $env:GOARCH = 'windows', $Arch
try {
    & (Join-Path $PSScriptRoot 'build-exe.ps1') -Out (Join-Path $stage 'aex.exe')
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
} finally {
    $env:GOOS, $env:GOARCH = $oldGOOS, $oldGOARCH
}

Copy-Item (Join-Path $root 'LICENSE'), (Join-Path $root 'NOTICE') $stage
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
Remove-Item -Recurse -Force $stage
Write-Host "Made $zip"
