# Building aex

How to build `aex.exe`, `aex-cli.exe` and the plugins from source. For what aex does, see [README.md](README.md); for
how the code works, [AGENTS.md](AGENTS.md).

## Build

Needs [Go](https://go.dev/dl/) (the version in `go.mod`) and git. No CGO, Node or Wails CLI: the
frontend is plain files embedded as they are, and `go build` makes the whole exe.

```powershell
git clone https://github.com/MaratBR/aex-tools.git
cd aex-tools
.\bin\build-exe.ps1            # dist\aex.exe, dist\aex-cli.exe, dist\plugins\*.exe
go test ./...                  # optional
```

Without PowerShell, or from macOS/Linux (cross-compiles; `GOARCH=arm64` for ARM Windows), the same
build by hand:

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o dist/plugins/ ./plugins/...
HASHES=$(for d in plugins/*/; do sha256sum "dist/plugins/$(basename "$d").exe" | cut -d' ' -f1; done | paste -sd,)
LDFLAGS="-s -w -X aex/internal/plugin.preApproved=$HASHES -X aex/internal/about.builtAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
GOOS=windows GOARCH=amd64 go build -tags desktop,production -trimpath -ldflags "$LDFLAGS -H windowsgui" -o dist/aex.exe .
GOOS=windows GOARCH=amd64 go build -tags desktop,production -trimpath -ldflags "$LDFLAGS" -o dist/aex-cli.exe .
```

- `aex.exe` is built with `-H windowsgui`: Windows starts it without a console, so opening the window
  flashes none. `aex-cli.exe` is the same program as a console one, for the terminal.

- Plugins go first: aex is built with their SHA-256 as pre-approved hashes (`-X aex/internal/plugin.preApproved`),
  so they run without asking to approve them. Leave it out and each asks once, as any plugin does.

- `-X aex/internal/about.builtAt` is the build time Settings > About shows; left out, it shows the exe
  file's time. The commit comes from git by itself (Go stamps it), so build from the checkout.
- `-tags desktop,production` is needed for the window (Wails; without it no window opens); plugins do not need it.
- `-trimpath` makes it a release build (see Release exe); leave it out for a dev build.
- The committed `rsrc_windows_*.syso` give the icon and version info; `go generate ./...` is needed only
  after changing them (see Release exe).

## With Docker (optional)

Builds in a `golang` container, without Go on the machine (Docker Desktop with Linux containers). From
the repo folder in PowerShell:

```powershell
docker run --rm -v "${PWD}:/src" -v aex-gomod:/go/pkg/mod -w /src `
  -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 golang:1.26 `
  sh -c 'git config --global --add safe.directory /src && go build -trimpath -ldflags=-s\ -w -o dist/plugins/ ./plugins/... && H=$(for d in plugins/*/; do sha256sum dist/plugins/$(basename $d).exe | cut -c1-64; done | paste -sd,) && go build -tags desktop,production -trimpath -ldflags=-s\ -w\ -X\ aex/internal/plugin.preApproved=$H\ -X\ aex/internal/about.builtAt=$(date -u +%Y-%m-%dT%H:%M:%SZ) -o dist/aex-cli.exe . && go build -tags desktop,production -trimpath -ldflags=-s\ -w\ -H\ windowsgui\ -X\ aex/internal/plugin.preApproved=$H\ -X\ aex/internal/about.builtAt=$(date -u +%Y-%m-%dT%H:%M:%SZ) -o dist/aex.exe .'
```

(In a POSIX shell: `-v "$PWD:/src"` and `\` for line breaks.)

- The output lands in `dist\` as with `build-exe.ps1`. Close the window first: unlike the script, this
  cannot move a running `dist\aex.exe` aside.
- `safe.directory`: the repo mounted in the container belongs to another user, and git (which Go asks for
  the commit it stamps into the exe) refuses it otherwise.
- The `aex-gomod` volume keeps downloaded modules between builds; `docker volume rm aex-gomod` drops it.
- `-ldflags=-s\ -w` rather than `-ldflags '-s -w'`: Windows PowerShell 5.1 drops the inner quotes when it
  passes the command to `docker`.

## Dev builds

```powershell
.\bin\aex.ps1                  # the window: pick a tool, run it
.\bin\aex.ps1 quota            # run one tool in the terminal
.\bin\aex.ps1 --plain quota    # its questions line by line instead of arrow-key prompts (also AEX_TUI=0)
.\bin\aex.ps1 --data-dir D:\aex-data
.\bin\worklog-sync.ps1 --help
# or
go run -tags desktop,production . worklog-sync --help
```

Builds need the Wails build tags `desktop,production` (the scripts pass them); without them the window
does not open.

The scripts build a dev `aex-cli.exe` into `dist\dev\` on each run (fast when nothing changed) and run it from the
current folder. Put `bin\` on `PATH` to call `worklog-sync.ps1` from anywhere.

## Release exe

`.\bin\build-exe.ps1` builds `dist\aex.exe` and `dist\aex-cli.exe` (~19 MB each; they need the WebView2
runtime, part of Windows 11). Double-click `aex.exe` to open the window; run `aex-cli.exe <tool> [args]` in
a terminal (without a tool it opens the window too). Given a tool, `aex.exe` only says to use
`aex-cli.exe` (in the terminal it was started from, else in a message box) and exits with 2.

- Shortcuts and starting on login point at `aex.exe`, also when set up from `aex-cli.exe`.

- `.env` is embedded at build time. A `.env` next to the exe is also read.
- Everything else goes to the data folder. Run `configure` first, or missing settings are prompted for on first use.
- Dev vs release: a build without `-trimpath` (the `bin\*.ps1` scripts, `go run`) reads `.env` from
  the repo; `build-exe.ps1` uses `-trimpath`, so the exe reads it from its own folder.
- The manifest (`winres\aex.manifest`, in both exes) sets `consoleAllocationPolicy` to `detached`: started
  from Explorer or the Start menu, `aex-cli.exe` opens no console window (Windows 11 24H2+; older Windows closes it right away);
  started from a terminal it runs there as usual.
- Icon and version info come from the committed `rsrc_windows_*.syso`. After changing `assets\logo.ico` or the
  version in `winres\winres.json`, run `go generate ./...` (uses [go-winres](https://github.com/tc-hib/go-winres)).
  Each plugin has its own `plugins\<name>\winres\winres.json` and `.syso`; a new plugin copies one and adds the
  `//go:generate` line. Author (Marat B, marat01q@gmail.com) is set in `CompanyName`, `LegalCopyright` and `Author`.
  To re-render `logo.ico` from `logo.svg`, use any SVG renderer with sizes 16, 24, 32, 48, 64, 128, 256.
  Explorer caches icons per path, so an old icon may linger for `dist\aex.exe` until the cache refreshes.
- Plugins are built to `plugins\<name>.exe` next to the exe (`dist\plugins\`, or `dist\dev\plugins\`
  for the scripts' dev build), before the exe: their SHA-256 are built into it as pre-approved, so they run
  without asking to approve them (see Plugins in [AGENTS.md](AGENTS.md)). A plugin rebuilt on its own asks again.
- `.\bin\build-exe.ps1 -Out <file>` builds elsewhere, e.g. while `dist\aex.exe` is running (it cannot be
  replaced then).

## Distribution

`.\bin\dist.ps1` runs `go vet ./...` and `go test ./...`, builds the release exe and plugins (as
`build-exe.ps1`), builds the installer from them with `LICENSE` and `NOTICE` (`installer\aex.iss`), and
zips `aex-<version>-windows-<arch>-setup.exe` with `SHA256SUMS` (the installer, `aex.exe`, `aex-cli.exe`
and `plugins/*.exe`, in `sha256sum -c` format) to `dist\aex-<version>-windows-<arch>.zip` (version from
`ProductVersion` in `winres\winres.json`). `-Arch arm64` for ARM Windows (default `amd64`), `-SkipTests`
to skip vet and tests. Stops at the first failure.

Needs [Inno Setup 6](https://jrsoftware.org/isinfo.php) (`winget install JRSoftware.InnoSetup`): `ISCC.exe`
on `PATH` or in its usual install folder.

The installer installs for the current user (no admin) into `%LOCALAPPDATA%\Programs\aex`, adds aex to
the Start menu (and, if picked, the desktop), and can open aex at the end. Silent: `/VERYSILENT`,
`/DIR=<folder>`. Uninstalling (Settings > Apps) removes the files, the shortcuts and starting on login;
the data folder stays.

## Tests

`go test ./...`

## Licenses of dependencies

`internal/about/about.json` holds aex's version, license and the licenses of everything built into it,
for Settings > About. After changing dependencies (`go.mod`), vendored files, `LICENSE`, `NOTICE` or the
version in `winres\winres.json`, run `go generate ./internal/about`; a test fails while it is out of date.
It also fails on a license it does not recognise or that Apache-2.0 aex cannot include (see AGENTS.md).
