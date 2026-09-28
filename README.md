# aex-tools

Personal work tooling in Go: one `aex` binary with a menu over all tools, each also runnable
directly (`aex <tool> [args]`) or from PowerShell (`bin\<tool>.ps1`).

## Layout

- `main.go` — entry point: global args, tool list, dispatch; embeds `.env`
- `menu.go` — menu shared parts (AEXT login header, plain numbered list, running a tool)
- `menu_tui.go` — full-screen arrow-key menu ([Bubble Tea](https://github.com/charmbracelet/bubbletea),
  alternate screen). Tools themselves run in the normal screen, so their output stays in scrollback.
  Falls back to the numbered list with `--plain`, `AEX_TUI=0`, or when stdin/stdout is not a terminal.
  The header shows who is logged in to AEXT (`/api/auth/me`, never prompts), settings and the data folder.
- `tool_<name>.go` — one file per tool
- `internal/`
  - `settings` — app root (repo or exe folder), data folder (`--data-dir`), output paths; loads `.env`,
    `.env.private`, app settings (`.env.config`); auth settings list, prompts for missing ones and saves
    them to app settings; `HoursPerDay()` / `TZOffsetHours()`, Jira→CSV project map, calendar country,
    leave statuses (`config.go`)
  - `dates` — day math and range expressions
  - `jira`, `aext` — API clients
  - `httpx` — on any unexpected response (status, non-JSON, wrong shape) dumps status, URL,
    relevant headers (cookie values redacted) and body (capped at 300 KB) to stderr
  - `quota` — monthly quota from AEXT working days, leaves and logged hours
  - `ui` — ANSI colors (only on a terminal; `NO_COLOR=1` disables, `FORCE_COLOR=1` forces), line
    prompts, hidden input, key press
- `bin\<name>.ps1` — PowerShell entrypoint per tool (thin wrapper over `bin\_invoke.ps1`)
- `bin\build-exe.ps1` — builds the release `dist\aex.exe`
- `assets\logo.svg` — icon source; `assets\logo.ico` rendered from it (16–256 px)
- `winres\winres.json`, `rsrc_windows_*.syso` — exe icon and version info

## Data folder

Per-user app data, by default `%APPDATA%\aex` (macOS: `~/Library/Application Support/aex`,
Linux: `$XDG_CONFIG_HOME/aex` or `~/.config/aex`). Holds:

- `.env.config` — app settings: auth settings saved by `configure` or prompts
- `session.txt` — AEXT session cookie (`AEXT_SESSION_FILE` overrides just this file)
- `output\jira-export\` — generated files

Change it with `--data-dir <dir>` (accepted before or after the tool name) or the `AEX_DATA_DIR`
environment variable (real environment only). `data-folder` opens it in Explorer.

## worklog-sync

Exports Jira worklogs to `<data folder>\output\jira-export\<timestamp>.csv` (`date,project,hours,description`,
one row per day + issue), then offers to send it to AEXT.

Range: by default suggested from AEXT (first working day this month with no hours per the AEXT working-days calendar, through today;
offers an earlier start if last month has gaps). Decline, or use `--manual`, to type one:
`today`, `yesterday`, `this week`, `last week`, `this month`, `last month`, `2026.09.20`,
`26.09.20`, `09.20`, `09.20-09.30`. `--range <expr>` skips the prompts.

AEXT login is by emailed code. The session cookie is cached in `session.txt` in the data folder.
Parallel requests share one login.

The CSV is re-read before import, so it can be edited before confirming. After import the quota is shown.

## quota

`.\bin\quota.ps1` shows last and current month: working days (AEXT calendar, `WorkingDaysCountry`),
expected hours (`HOURS_PER_DAY` app setting), logged, % filled, hours behind as of today, and hours/day needed
over the remaining working days (today included).

Leaves (`/api/leaves/my-requests`, fetched per calendar year) remove their working days from the quota
and from gap detection in `worklog-sync`. `declined` and `cancelled` leaves are ignored with a warning
(`IgnoredLeaveStatuses` in `internal/settings/config.go`); `pending` (or any other non-`approved` status) is counted, with a warning;
a leave created by someone other than you (by email) is also reported. Leave records are mostly
redacted, so only ids, emails, dates, type and status are used.

## configure

`.\bin\configure.ps1` asks for the auth settings `AEXT_EMAIL`, `JIRA_EMAIL` and `JIRA_TOKEN` (hidden input),
then `HOURS_PER_DAY` and `TZ_OFFSET_HOURS` (validated), and saves them to app settings (`.env.config` in the
data folder). Empty answer keeps the current value, `-` removes it from app settings (or resets it to its default). Warns when the value is also set in a fallback source, since app settings
will override it. Offers to log out of AEXT when the email changes.

Tools that need a missing auth setting prompt for it and offer to save it to app settings too.

## data-folder

`.\bin\data-folder.ps1` prints the data folder and opens it in the file manager (`--print`: only print).

## Usage

Needs Go (see `go.mod`) for the scripts and builds; the built exe needs nothing.

```powershell
.\bin\aex.ps1                  # menu: pick a tool, run it, back to the menu until q
.\bin\aex.ps1 --plain          # numbered-list menu instead of the arrow-key UI (also AEX_TUI=0)
.\bin\aex.ps1 quota            # run one tool
.\bin\aex.ps1 --data-dir D:\aex-data
.\bin\worklog-sync.ps1 --help
# or
go run . worklog-sync --help
```

The scripts build a dev exe into `dist\dev\` on each run (fast when nothing changed) and run it from the
current folder. Put `bin\` on `PATH` to call `worklog-sync.ps1` from anywhere.

## Executable

`.\bin\build-exe.ps1` builds `dist\aex.exe` (~8 MB, no runtime needed). Double-click it to open a terminal
with the menu, or run `aex.exe <tool> [args]`.

- `.env` is embedded at build time. A `.env` or `.env.private` next to the exe is also read.
- Everything else goes to the data folder. Run `configure` first, or missing settings are prompted for on first use.
- Dev vs release: a build without `-trimpath` (the `bin\*.ps1` scripts, `go run`) reads `.env` and
  `.env.private` from the repo; `build-exe.ps1` uses `-trimpath`, so the exe reads them from its own folder.
- Icon and version info come from the committed `rsrc_windows_*.syso`. After changing `assets\logo.ico` or the
  version in `winres\winres.json`, run `go generate` (uses [go-winres](https://github.com/tc-hib/go-winres)).
  To re-render `logo.ico` from `logo.svg`, use any SVG renderer with sizes 16, 24, 32, 48, 64, 128, 256.
  Explorer caches icons per path, so an old icon may linger for `dist\aex.exe` until the cache refreshes.
- `.\bin\build-exe.ps1 -Out <file>` builds elsewhere, e.g. while `dist\aex.exe` is running (it cannot be
  replaced then).

## Adding a tool

1. Create `tool_foo.go` with `func foo(args []string) error`; parse args with `parseFlags` (gives `--help`).
2. Add it to `tools` in `main.go`.
3. Copy `bin\worklog-sync.ps1` to `bin\foo.ps1`, change the name.

## Tests

`go test ./...`

## Config

Lowest to highest priority:

- `.env` (repo / next to the exe): shared non-secret config (base URLs), committed (built into the exe).
- `.env.private` (repo / next to the exe): gitignored fallback for auth settings (template: `.env.private.example`).
- Real environment variables.
- App settings, `.env.config` (data folder): where auth settings belong. Written by `configure` and by
  missing-setting prompts. Overrides everything, including real environment variables.

App settings with defaults, written to `.env.config` on first start (keeping a valid value already set in a
fallback source). Invalid values fall back to the default with a warning. Read live, so `configure` changes
apply in the running menu:

- `HOURS_PER_DAY` (default `8`) — working hours per day for the quota.
- `TZ_OFFSET_HOURS` (default `7`) — UTC offset all dates are computed in (ranges, "today", worklog days,
  file timestamps); quarter hours allowed, e.g. `5.5` = UTC+5:30.

The menu header shows the current values, the settings file path and the data folder.

`.claude/settings.json` denies Claude Code access to `.env.private`, `.env.config` and `session.txt`.
