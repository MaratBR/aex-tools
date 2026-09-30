# Building aex

How to build `aex.exe` and its plugins from source. For what aex does, see [README.md](README.md); for
how the code works, [AGENTS.md](AGENTS.md).

## Build

Needs [Go](https://go.dev/dl/) (the version in `go.mod`) and git. No CGO, Node or Wails CLI: the
frontend is plain files embedded as they are, and `go build` makes the whole exe.

```powershell
git clone https://github.com/MaratBR/aex-tools.git
cd aex-tools
.\bin\build-exe.ps1            # dist\aex.exe and dist\plugins\*.exe
go test ./...                  # optional
```

Without PowerShell, or from macOS/Linux (cross-compiles; `GOARCH=arm64` for ARM Windows), the same
build by hand:

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o dist/plugins/ ./plugins/...
HASHES=$(for d in plugins/*/; do sha256sum "dist/plugins/$(basename "$d").exe" | cut -d' ' -f1; done | paste -sd,)
GOOS=windows GOARCH=amd64 go build -tags desktop,production -trimpath \
  -ldflags "-s -w -X aex/internal/plugin.preApproved=$HASHES" -o dist/aex.exe .
```

- Plugins go first: aex is built with their SHA-256 as pre-approved hashes (`-X aex/internal/plugin.preApproved`),
  so they run without asking to approve them. Leave it out and each asks once, as any plugin does.

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
  sh -c 'go build -trimpath -ldflags=-s\ -w -o dist/plugins/ ./plugins/... && H=$(for d in plugins/*/; do sha256sum dist/plugins/$(basename $d).exe | cut -c1-64; done | paste -sd,) && go build -tags desktop,production -trimpath -ldflags=-s\ -w\ -X\ aex/internal/plugin.preApproved=$H -o dist/aex.exe .'
```

(In a POSIX shell: `-v "$PWD:/src"` and `\` for line breaks.)

- The output lands in `dist\` as with `build-exe.ps1`. Close the window first: unlike the script, this
  cannot move a running `dist\aex.exe` aside.
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

The scripts build a dev exe into `dist\dev\` on each run (fast when nothing changed) and run it from the
current folder. Put `bin\` on `PATH` to call `worklog-sync.ps1` from anywhere.

## Release exe

`.\bin\build-exe.ps1` builds `dist\aex.exe` (~19 MB; needs the WebView2 runtime, part of Windows 11).
Double-click it to open the window, or run `aex.exe <tool> [args]` in a terminal.

- `.env` is embedded at build time. A `.env` next to the exe is also read.
- Everything else goes to the data folder. Run `configure` first, or missing settings are prompted for on first use.
- Dev vs release: a build without `-trimpath` (the `bin\*.ps1` scripts, `go run`) reads `.env` from
  the repo; `build-exe.ps1` uses `-trimpath`, so the exe reads it from its own folder.
- The manifest (`winres\aex.manifest`) sets `consoleAllocationPolicy` to `detached`: started from Explorer or
  the Start menu, the exe opens no console window (Windows 11 24H2+; older Windows closes it right away);
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

## Tests

`go test ./...`
