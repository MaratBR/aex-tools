# Makes the Windows distribution: vets and tests, builds the release exe and plugins (bin\build-exe.ps1),
# builds the installer from them (installer\aex.iss, Inno Setup 6), then zips the installer with
# SHA256SUMS (the installer, aex.exe, aex-cli.exe and each plugin) to dist\aex-<version>-windows-<arch>.zip.
# -Arch amd64 (default) or arm64. -SkipTests skips go vet and go test.
param([ValidateSet('amd64', 'arm64')][string]$Arch = 'amd64', [switch]$SkipTests)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')

# Inno Setup's compiler: on PATH, else where its installer (or winget) puts it.
$iscc = (Get-Command ISCC.exe -ErrorAction SilentlyContinue).Source
if (-not $iscc) {
    $iscc = @("${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe", "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe") | Where-Object { Test-Path $_ } | Select-Object -First 1
}
if (-not $iscc) { throw 'Inno Setup 6 not found (ISCC.exe): winget install JRSoftware.InnoSetup' }

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
$build = Join-Path $stage 'build'
$zip = Join-Path $dist "$name.zip"
$setup = "$name-setup"

if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force $build | Out-Null

$oldGOOS, $oldGOARCH = $env:GOOS, $env:GOARCH
$env:GOOS, $env:GOARCH = 'windows', $Arch
try {
    & (Join-Path $PSScriptRoot 'build-exe.ps1') -Out (Join-Path $build 'aex.exe')
    if ($LASTEXITCODE) { exit $LASTEXITCODE }
} finally {
    $env:GOOS, $env:GOARCH = $oldGOOS, $oldGOARCH
}
Copy-Item (Join-Path $root 'LICENSE'), (Join-Path $root 'NOTICE') $build

& $iscc /Q "/DVersion=$version" "/DArch=$Arch" "/DSource=$build" "/DOutDir=$stage" "/DOutName=$setup" (Join-Path $root 'installer\aex.iss')
if ($LASTEXITCODE) { exit $LASTEXITCODE }

# SHA256SUMS in sha256sum's format (sha256sum -c checks it): the installer, then the files it installs.
$files = @(Get-Item (Join-Path $stage "$setup.exe"), (Join-Path $build 'aex.exe'), (Join-Path $build 'aex-cli.exe')) +
    @(Get-ChildItem (Join-Path $build 'plugins') -Filter *.exe | Sort-Object Name)
$sums = foreach ($f in $files) {
    $rel = if ($f.DirectoryName -eq (Join-Path $build 'plugins')) { "plugins/$($f.Name)" } else { $f.Name }
    "$((Get-FileHash -Algorithm SHA256 $f.FullName).Hash.ToLower())  $rel"
}
[IO.File]::WriteAllText((Join-Path $stage 'SHA256SUMS'), (($sums -join "`n") + "`n"))

if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $stage "$setup.exe"), (Join-Path $stage 'SHA256SUMS') -DestinationPath $zip
Remove-Item -Recurse -Force $stage
Write-Host "Made $zip"
