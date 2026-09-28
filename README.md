# aex-tools

Personal work tooling. Node scripts (no build step, ESM, Node >= 20.12), each runnable from PowerShell,
or all together from one menu / a single `aex.exe`.

## Layout

- `scripts/<name>.js` — one script per tool
- `scripts/aex.js` — menu / dispatcher over all tools (the exe entry point). The menu is a
  full-screen arrow-key UI (`lib/tui.js`, alternate screen) on a terminal; tools themselves run in the
  normal screen, so their output stays in scrollback. Falls back to a numbered list with `--plain`,
  `AEX_TUI=0`, or when stdin/stdout is not a terminal. The header shows who is logged in to AEXT
  (`/api/auth/me`, never prompts) and the data folder.
- `scripts/build-exe.js` — builds `dist/aex.exe`
- `lib/` — shared helpers
  - `paths.js` — exe detection, repo/exe folder, data folder (`--data-dir`), output paths
  - `config.js` — `hoursPerDay()` / `tzOffsetHours()` from app settings, Jira→CSV project map, calendar
    country, leave statuses; re-exports `paths.js`
  - `env.js` — loads `.env`, `.env.private`, app settings (`.env.config`); auth settings list
    (`AUTH_SETTINGS`), prompts for missing ones and saves them to app settings
  - `dates.js` — day math and range expressions
  - `jira.js`, `aext.js` — API clients
  - `color.js` — ANSI colors, only on a terminal; `NO_COLOR=1` disables, `FORCE_COLOR=1` forces
  - `http.js` — on any unexpected response (status, non-JSON, wrong shape) dumps status, URL,
    relevant headers (cookie values redacted) and body (capped at 300 KB) to stderr
  - `tui.js`, `prompt.js`, `cli.js`, `csv.js`
- `bin/<name>.ps1` — PowerShell entrypoint per script (thin wrapper over `bin/_invoke.ps1`)
- `assets/logo.svg` — exe icon

## Data folder

Per-user app data, by default `%APPDATA%\aex` (macOS: `~/Library/Application Support/aex`,
Linux: `$XDG_CONFIG_HOME/aex` or `~/.config/aex`). Holds:

- `.env.config` — app settings: auth settings saved by `configure` or prompts
- `session.txt` — AEXT session cookie (`AEXT_SESSION_FILE` overrides just this file)
- `output\jira-export\`, `output\repo-zip\` — generated files

Change it with `--data-dir <dir>` (accepted by every script and the exe, before or after the tool name)
or the `AEX_DATA_DIR` environment variable (real environment only). `data-folder` opens it in Explorer.

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

`.\bin\quota.ps1` shows last and current month: working days (AEXT calendar, `WORKING_DAYS_COUNTRY`),
expected hours (`HOURS_PER_DAY` app setting), logged, % filled, hours behind as of today, and hours/day needed
over the remaining working days (today included).

Leaves (`/api/leaves/my-requests`, fetched per calendar year) remove their working days from the quota
and from gap detection in `worklog-sync`. `declined` and `cancelled` leaves are ignored with a warning
(`IGNORED_LEAVE_STATUSES` in `lib/config.js`); `pending` (or any other non-`approved` status) is counted, with a warning;
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

## zip-repo

`.\bin\zip-repo.ps1` zips the files committed at HEAD (`git archive`) into
`<data folder>\output\repo-zip\<repo>-<commit>-<timestamp>.zip`. Uncommitted, staged, untracked and
gitignored files are left out.

## Usage

```powershell
.\bin\aex.ps1                  # menu: pick a tool, run it, back to the menu until q
.\bin\aex.ps1 --plain          # numbered-list menu instead of the arrow-key UI (also AEX_TUI=0)
.\bin\aex.ps1 quota            # run one tool
.\bin\aex.ps1 --data-dir D:\aex-data
.\bin\worklog-sync.ps1 --help
# or
npm run worklog-sync -- --help
```

Put `bin\` on `PATH` to call `worklog-sync.ps1` from anywhere.

## Executable

`npm install`, then `npm run build:exe` builds `dist\aex.exe` (~70 MB): a Node
[single executable application](https://nodejs.org/api/single-executable-applications.html) with node and
all scripts inside, no Node install needed to run it. Double-click it to open a terminal with the menu,
or run `aex.exe <tool> [args]`. zip-repo is left out (it needs the git checkout).

- `.env` is embedded at build time. A `.env` or `.env.private` next to the exe is also read.
- Everything else goes to the data folder. Run `configure` first, or missing settings are prompted for on first use.
- Build uses the running Node for the exe; rebuild after changing scripts or `.env`.
- Icon comes from `assets/logo.svg` (rendered to a multi-size .ico at build), version info from `package.json`.
  Explorer caches icons per path, so an old icon may linger for `dist\aex.exe` until the cache refreshes.
- `npm run build:exe -- --out <file>` builds elsewhere, e.g. while `dist\aex.exe` is running (it cannot be
  replaced then).

## Adding a script

1. Create `scripts/foo.js` exporting `summary` and `async function main(argv)`, ending with
   `if (isEntry(import.meta.url)) run(main);` (`lib/cli.js`).
2. Add it to `TOOLS` in `scripts/aex.js`.
3. Copy `bin/worklog-sync.ps1` to `bin/foo.ps1`, change the name.
4. Add `"foo": "node scripts/foo.js"` to `package.json` scripts.

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
