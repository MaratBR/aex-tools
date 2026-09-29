# aex-tools

Personal work tooling in Go: one `aex` binary that opens a window with all tools, each also runnable
directly in the terminal (`aex <tool> [args]`) or from PowerShell (`bin\<tool>.ps1`).

## Layout

- `main.go` — entry point: global args, tool registry, dispatch; embeds `.env`. No tool opens the window.
- `gui.go` — opens the window (`internal/webui`) over the tools; `gui_windows.go` closes the console
  Windows gives the exe when it is started from the Start menu or Explorer
- `header.go` — the window's header: who is logged in to AEXT (`/api/auth/me`) and Jira
  (`/rest/api/3/myself`), both checked in parallel and never prompting, then settings and the data folder;
  running a tool (`execTool`)
- `internal/`
  - `tool` — `Tool` type every tool exports (name, summary, run, or sub-tools for a group) and
    `ParseFlags` (gives `--help`)
  - `tools/<name>` — one package per built-in tool (`worklogsync`, `quota`, `account`, `configure`,
    `plugins`, `customtools`), each exporting `Tool`; `main.go` lists them in the order the window shows them
  - `plugin` — plugins (see Plugins): finds them, checks their safe hash, runs them; `plugin.Main` for
    a plugin's own `main`; `plugin.Lock` approves custom tools the same way
  - `custom` — custom tools (see Custom tools): the list of scripts added, their approval, parameter
    prompts, running them through their adapter
  - `adapter` — the tool adapter interface and the parameter model every adapter shares (binding args,
    `--help`); `adapter/powershell` — the PowerShell one
  - `gitinfo` — a file's git repo, commit, last commit that changed it and its changes not committed
  - `proc` — starting console programs without a console window (from the window)
  - `pty` — running a console program in a pseudo-console (ConPTY) the window shows as a terminal
  - `settings` — app root (repo or exe folder), data folder (`--data-dir`), output paths; loads `.env`,
    app settings (`.env.config`), the credential store; auth settings list, prompts for missing ones and saves
    them to app settings; `HoursPerDay()` / `TZOffsetHours()`, Jira→CSV project map, calendar country,
    leave statuses (`config.go`)
  - `shortcut` — adds aex to the Start menu (Windows), `~/Applications` (macOS) or the app menu (Linux)
    and removes it; asks once on first launch
  - `dates` — day math and range expressions
  - `jira`, `aext` — API clients
  - `httpx` — on any unexpected response (status, non-JSON, wrong shape) dumps status, URL,
    relevant headers (cookie values redacted) and body (capped at 300 KB) to stderr
  - `quota` — monthly quota from AEXT working days, leaves and logged hours
  - `ui` — ANSI colors (only on a terminal; `NO_COLOR=1` disables, `FORCE_COLOR=1` forces), line
    prompts, hidden input, key press; `ui.Remote` answers all prompts elsewhere (the window)
  - `webui` — the window ([Wails](https://wails.io), the system WebView2 on Windows): see Window
- `plugins/<name>` — plugin sources (`main` packages), built to `plugins\<name>.exe` next to the exe
- `bin\<name>.ps1` — PowerShell entrypoint per tool (thin wrapper over `bin\_invoke.ps1`)
- `bin\build-exe.ps1` — builds the release `dist\aex.exe`
- `assets\logo.svg` — icon source; `assets\logo.ico` rendered from it (16–256 px)
- `winres\winres.json`, `rsrc_windows_*.syso` — exe icon and version info

## Data folder

Per-user app data, by default `%APPDATA%\aex` (macOS: `~/Library/Application Support/aex`,
Linux: `$XDG_CONFIG_HOME/aex` or `~/.config/aex`). Holds:

- `.env.config` — app settings: auth settings saved by `configure` or prompts
- `credentials.json` — AEXT session and `JIRA_TOKEN`, only when there is no OS credential store (see Credentials)
- `output\jira-export\` — generated files
- `custom-tools.json` — the custom tools added (name, script path, adapter)

Change it with `--data-dir <dir>` (accepted before or after the tool name) or the `AEX_DATA_DIR`
environment variable (real environment only). Click its path in the window's sidebar to open it.

## worklog-sync

Exports Jira worklogs to `<data folder>\output\jira-export\<timestamp>.csv` (`date,project,hours,description`,
one row per day + issue), then offers to send it to AEXT.

Before sending, rows AEXT already has (`GET /api/time-entries/`: same date, project and description) are
skipped and listed, with both hours when they differ; so are rows repeated in the CSV.

Range: by default suggested from AEXT (first working day this month with no hours per the AEXT working-days calendar, through today;
offers an earlier start if last month has gaps). Decline, or use `--manual`, to type one:
`today`, `yesterday`, `this week`, `last week`, `this month`, `last month`, `2026.09.20`,
`26.09.20`, `09.20`, `09.20-09.30`. `--range <expr>` skips the prompts.

AEXT login is by emailed code. When `AEXT_EMAIL` is not set but a saved session is still valid, the email
AEXT reports for it (`/api/auth/me`, checked by the window header and `account`) is used and saved to app settings. The session cookie is kept in the credential store (see Credentials).
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

## account

`.\bin\account.ps1` shows who you are logged in as: AEXT (`/api/auth/me`: name, email, other profile fields,
server, session file) and Jira (`/rest/api/3/myself`: name, email, account id, time zone, …), plus the login
settings and where each comes from (token masked). On a terminal it then offers actions until Done:

- log in to AEXT (new emailed code, replaces the cached session)
- log out of AEXT: `POST /api/auth/logout` (expects 204), then deletes the saved session
- wipe the AEXT session: deletes the saved session only, the server session stays valid until it expires
- set or change the Jira login: email + API token, checked against Jira before saving to app settings
- remove the Jira token from app settings

Logout, wipe, removing the token and replacing a login ask for confirmation (default no). Flags run one
action directly: `--show`, `--login aext|jira`, `--logout aext|jira`, `--wipe-session`.

## configure

In the window, settings are a form rather than a tool: the gear next to the logo (or the Settings line in the
sidebar, or typing `configure`) opens it. It edits every setting below at once (checked before anything is
saved; an empty field removes the setting, or resets it to its default), toggles the app launcher entry, and
wipes after you type `CONFIRM`. The rest of this section is the terminal tool, `aex configure`.

`.\bin\configure.ps1` lists the settings with their current values: the auth settings `AEXT_EMAIL`,
`JIRA_EMAIL` and `JIRA_TOKEN` (hidden input), then `HOURS_PER_DAY` and `TZ_OFFSET_HOURS` (validated). Pick one
to change it; it is saved to app settings (`.env.config` in the data folder) right away, then the list comes
back until Done. When some have no value, "Fill in the N not set" asks for just those.
`configure NAME` asks for one setting; `configure NAME=value` saves it without asking (not for `JIRA_TOKEN`).
Empty answer keeps the current value, `-` removes it from app settings (or resets it to its default). Warns when
the value is also set in a fallback source, since app settings will override it. Offers to wipe the AEXT
session when the email changes.

The `configure` list (or `configure --wipe-settings` / `--wipe-all`) also wipes, after listing what goes and asking you to
type `CONFIRM`:
- Wipe all settings: `.env.config`, secret settings (`JIRA_TOKEN`) in the credential store and
  `plugin-settings/`. App settings get their defaults back.
- Wipe everything: all settings, the AEXT session, and every entry of the data folder (`output/`, plugin data,
  anything else). Plugins and their approvals are kept. Refused when the data folder holds the app.

`.env` and environment variables are never touched and still apply as fallbacks.

The `configure` list (or `configure --add-shortcut` / `--remove-shortcut`) also adds aex to the OS app launcher, pointing at
the running exe (with `--data-dir` if it was given), or removes it:
- Windows: `aex.lnk` in the Start menu (`%APPDATA%\Microsoft\Windows\Start Menu\Programs`), with the exe's icon.
- macOS: `~/Applications/aex.app`, a small bundle that opens the exe in Terminal. Only a bundle aex made is removed.
- Linux: `aex.desktop` in `$XDG_DATA_HOME/applications` (default `~/.local/share/applications`), run in a terminal.

The first time a release build opens the window, it asks once whether to add it (skipped when it is already
there); the answer is remembered as `SHORTCUT_ASKED` in app settings, so wiping settings asks again.

Tools that need a missing auth setting prompt for it and offer to save it to app settings too.

## plugins

`.\bin\plugins.ps1` manages plugins (see Plugins). Without args on a terminal it lists them and asks
what to do; else `list` (default), `describe [<name>]` (asks to approve any not safe yet),
`delete <name> [--yes]` (deletes the file and forgets its safe hash and granted access), `forget-all [--yes]` (forgets every plugin's safe hash and
granted access, keeping the files: each asks to be approved again on its next run), `open` (opens the
plugins folder). The credential store cannot list its keys, so approved plugin paths are also kept in
`plugin-approved-paths`; `forget-all` clears those plus the plugins in the folder now. Custom tools'
approvals are in that list too, so `forget-all` forgets them as well.

## custom-tools

`.\bin\aex.ps1 custom-tools` manages custom tools (see Custom tools). Without args on a terminal (or
from the window) it lists them and asks what to do: add a script (asks its path and a name), describe or
remove one. Else `list` (default: file, SHA-256, state, adapter, parameters, git info), `describe <name>`
(that plus its `--help`), `add <path> [--name <name>]` (named after the file by default, e.g.
`Deploy App.ps1` → `Deploy-App`), `remove <name> [--yes]` (forgets its safe hash; the script is kept).

## cm-release (plugin)

A group of CM release tools (`.\bin\cm-release.ps1 <tool> [args]`). Needs Jira access (see Plugins).

The git tools work on every CM repo at once: `<reposDir>\<repo>` for each of the `repos` settings
(default `clearmechanic.frontend`, `clearmechanic.siteforappointments`, `cmos.datamigration`, `src`,
`cmos.microservices`). The repos folder is asked for on first run; both are in the settings menu of
`jira-handoff --settings`. Branches: `master`, `PROD`, `staging`, `release/VERSION`; tag `vVERSION`.

### pull-all

`.\bin\cm-release.ps1 pull-all` checks every repo is clean (asks to fix and recheck), then fetches
(`--all --prune`) and pulls master, PROD and staging in each, finishing on staging.

### prepare-release

`.\bin\cm-release.ps1 prepare-release [--version 4.5]` checks every repo is clean, then creates
`release/VERSION` off staging in each (VERSION asked for when left out). If all repos are already on the
same release branch, offers to use it. Shows each repo's `git diff --stat PROD...release/VERSION` one at a
time, then asks before pushing the branch to origin (checking each repo is on it and clean first).
No tag yet: merge-prod makes it.

### merge-prod

`.\bin\cm-release.ps1 merge-prod` detects VERSION from the `release/VERSION` branch all repos are on
(confirmed first), then validates each: fetched, on that branch, clean, local PROD equal to `origin/PROD`.
An existing `vVERSION` tag is shown (where it points, commit details) and, if you agree, deleted locally
and on origin. After confirming, merges into PROD (`--no-ff`) and tags the merge commit in each repo, then
asks whether to push PROD and the tag.

### jira-handoff

`.\bin\cm-release.ps1 jira-handoff [--epic CM-6996] [--dry-run] [--manual]` hands the epic's child tickets in
"Ready for Production" off to QA: comments "Please test in PROD" (To: QA owner, Cc: the always-Cc people
minus the QA owner; no Cc line when that leaves nobody), assigns the QA owner (`customfield_11215`; none
means the default QA), then moves the ticket to In Production (transition `111`, always last).

- Epic (`--epic` or asked): key (`CM-7042`), link (`https://…/browse/CM-7042`, `…?selectedIssue=CM-7042`)
  or bare number (`7042` = default prefix + number, shown and confirmed first). A pasted link turns into
  the key right away (typed one works too, it just stays as typed). As you type, the epic's summary
  shows under the box once the answer is an existing epic (nothing otherwise); Enter on anything else
  shows why it is refused (not found, not an epic).
- Settings (`--settings`, or "Settings" in the first question when run without `--epic`): default
  project prefix (`CM`), default QA (required), always-Cc people, excluded assignees, each excluded one set to "ask each run" or "skip
  automatically". People are found by name or email in Jira. Kept in
  `<data folder>\plugin-settings\cm-release.json` (moved there from `jira-release-handoff.json` if that exists);
  first run writes the defaults (prefix CM, QA Heriberto, Cc Alexander + Heriberto, Bulat excluded with ask).
- `[Mobile]` tickets are skipped; `[MobileAPI]` counts as Web. A ticket tagged both `[Mobile]` and
  `[Web]` / `[MobileAPI]` is asked about. Then excluded assignees' tickets are skipped or asked about
  (default no), per their setting.
- Release check: the epic's releases are its fix versions plus project releases named in its summary
  (`5.12` matches "Release 5.12", not "5.12.1"). One: shown, you approve or decline checking against it.
  Several: pick one or none. With a release that has a date, every ticket still to hand off (after all
  other rules) whose last move to Ready for Production (changelog, in `TZ_OFFSET_HOURS`) is after the
  release date gets a warning: allow this ticket, allow all like it, or decline (skip). Allowed ones are
  marked in the plan, summary and report.
- Several QA owners: asks which one. `--manual` (or yes to "Confirm each ticket?") asks per ticket.
- All questions come first; tickets without the In Production transition available are skipped. Then the
  plan is shown and you pick hand off, dry run or cancel. `--dry-run` changes nothing.
- Stops at the first failure without retrying (the comment may be posted already).
- Summary lists skipped tickets with links and why; a Markdown report goes to
  `<data folder>\output\cm-release\jira-handoff\<epic>-<timestamp>[-dry-run].md`.

## Usage

Needs Go (see `go.mod`) for the scripts and builds; the built exe needs nothing.

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

## Executable

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
  for the scripts' dev build).
- `.\bin\build-exe.ps1 -Out <file>` builds elsewhere, e.g. while `dist\aex.exe` is running (it cannot be
  replaced then).

## Window

`aex` without a tool opens the window: the tools on the left (a group opens under its name), the logins
and settings under them, and the runs on the right. Tools are the same console programs as in the
terminal:

- What a tool prints (stdout and stderr, plugins' too) goes through a pipe into its run, ANSI colors
  included. Each run shows its command and status (running, needs your answer, done or failed in N s).
- Its questions (`ui.Input`, `Confirm`, `Choose`, `WaitKey`) are asked inside its run, through `ui.Remote`,
  and answered there: a text field (Esc cancels), Yes / No (y / n), a list (1-9), Continue. `Validate` runs
  in Go; a rejected answer asks again with the reason. The answer stays in the run.
- A text question can say what kind of answer it takes with `ui.Field.Hint` (plugins' questions pass it
  on too): `ui.HintFile` (optionally with `Filters`, e.g. `{"PowerShell scripts", ["*.ps1"]}`) or
  `ui.HintFolder`. The window then adds "Choose file…" / "Choose folder…", opening the system dialog
  where the answer so far points, and a file dropped anywhere on the window fills the answer with its
  path. Terminals ignore the hint.
- Before a question or the end of a run, the output pipe is synced (a marker written through it), so
  nothing printed before a question shows up after it.
- The line at the bottom runs a typed command, e.g. `quota --verbose` (Tab completes tool names).
- One tool runs at a time; Clear empties the list of runs.

Frontend: plain HTML, CSS and JS in `internal/webui/frontend` (no build step), embedded in the exe.
Plugins' own questions are not answered in the window yet: a plugin sees no terminal there.

## Adding a tool

1. Create package `internal/tools/foo` exporting `var Tool = tool.Tool{Name: "foo", Summary: "...", Run: run}`;
   parse args with `tool.ParseFlags` (gives `--help`).
2. Add `foo.Tool` to `builtins` in `main.go`. (Or make it a plugin: see Plugins.)
3. Copy `bin\worklog-sync.ps1` to `bin\foo.ps1`, change the name.

A tool can instead be a group of tools: `tool.Tool{Name: "foo", Summary: "...", Sub: []tool.Tool{...}}`.
A group does nothing on its own; `aex foo <tool> [args]` (name or number) runs one of its tools, `aex foo`
asks which, `aex foo --help` lists them. The window lists its tools under it. Groups may nest.

## Plugins

A plugin is a tool in its own executable in the `plugins` folder next to `aex.exe`. It shows in the window
after the built-in tools and runs as `aex <name> [args]`, in the same console. aex passes it the data
folder, app root and built-in `.env` (`settings.PluginEnv`), so it sees the same settings, but not the
credentials: a plugin never opens the credential store and ignores secret settings (`JIRA_TOKEN`) from
`.env` files and the environment. It gets only the credentials of the access it asks for and you grant.

- Adding one in this repo: create `plugins/foo/main.go` (`package main`) calling
  `plugin.Main(tool.Tool{Name: "foo", Summary: "...", Run: run})`. The build scripts build every
  `plugins/*` into the plugins folder. Optionally copy `bin\worklog-sync.ps1` to `bin\foo.ps1`.
- A plugin can be a group of tools (see Adding a tool): `plugin.Main(tool.Tool{Name: "foo", Summary: "...",
  Sub: []tool.Tool{{Name: "bar", ...}}})`. Its tools are named plainly (`bar`, not `foo-bar`); aex runs
  one as `<plugin> bar [args]`. Access is the plugin's, for all its tools. The window shows them once the
  plugin is approved (before that it cannot be described, so it shows as one tool, which asks which of
  its tools to run).
- Any other executable works too if it answers `--aex-describe` with `{"summary": "..."}` on stdout,
  plus `"tools": [{"name": "...", "summary": "..."}]` (each maybe with its own `"tools"`) for a group.
- Access: a plugin asks for it with `plugin.Main(t, plugin.Jira)` (`"access": ["jira"]` in the describe
  JSON). Before running it aex asks once to allow it; the grant is saved for the file's SHA-256
  (`plugin-access:<path>`), so a changed file asks again. aex then makes sure the settings are set
  (prompting like any tool) and passes the secret ones as `AEX_PLUGIN_SECRET_<NAME>` env vars, which the
  plugin takes into an in-memory store and removes from its environment. `jira` = `JIRA_EMAIL` +
  `JIRA_TOKEN`; add kinds in `internal/plugin/access.go`. This limits honest plugins and shows what they
  use; it is no sandbox, a plugin still runs with your rights (hence approving its file).
- Nothing in the plugins folder runs, not even `--aex-describe`, until you approve its file. Until then
  the window shows only its file name, size and modified time, read without running it. Running it (or
  `plugins describe`) shows its path, size and SHA-256 and asks; on yes the SHA-256 of the file's
  contents is saved as its safe hash in the credential store (`plugin-safe-sha256:<path>`).
- Before every run, `--aex-describe` included, aex hashes the file again and asks again if it no longer
  matches the safe hash (e.g. after a rebuild with changed code). On Windows the file stays open
  without write/delete sharing from hashing until the process starts, so it cannot be swapped in between.
  Elsewhere it is not locked.
- Without a terminal, a plugin that is not safe fails instead of asking.

## Custom tools

A custom tool is a script anywhere on disk, added with `custom-tools add <path>`, run by the tool
adapter for its kind of file. It shows in the window after the plugins and runs as `aex <name> [args]`.
Adapters so far: `powershell` (`.ps1`). A new one implements `adapter.Adapter` (`internal/adapter`) and is
added to `custom.Adapters`:

- `Handles(path)` — whether it runs this file (by extension); `FileTypes()` — its files for the file
  dialog of `custom-tools add` (e.g. "PowerShell scripts", `*.ps1`).
- `Describe(path)` — summary and parameters, read without running the script: name, kind (string, int,
  number, bool, switch, list), declared type, required, default, choices, help, aliases, and a hint
  when a value is a file or folder path (asked for with the window's dialog and drop, see Window).
  PowerShell: `[IO.FileInfo]` / `[IO.DirectoryInfo]`, or a string named `…File` / `…Path` (file) or
  `…Folder` / `…Dir` / `…Directory` (folder).
- `Command(path, description, args, rest, session)` — the command that runs it with those values. The
  session says whether it has aex's terminal, or a terminal of its own in the window (see below).
- The description's `Interactive`: whether the script may ask questions while it runs (the adapter
  decides; PowerShell: always).

aex does the rest the same for every adapter:

- Args: `-Name value` or `--name value`, `-Name:value` or `--name=value`, a switch alone or `-Force:false`,
  lists comma-separated; names ignore case and may be shortened or be an alias; positional values fill the
  parameters not named yet, in order. Values are checked against the kind and choices before the script
  runs. `aex <name> --help` shows the parameters (no approval needed: nothing runs).
- Without args, every parameter is asked for (switches and bools as Yes / No, choices as a list, the rest
  as text; empty leaves an optional one to the script's default). With args, only required ones left out
  are asked for; without a terminal they fail instead.
- Approval as for plugins: the script's SHA-256 is its safe hash (`plugin-safe-sha256:<path>`), asked for
  on the first run and again whenever the file changes. The approval shows its path, size, SHA-256,
  adapter and what runs it, its parameters and its git info. The script is held open from hashing until it
  exits (on Windows it cannot change in between); files it dot-sources or imports are not covered.
- Git: when the script is in a git repo, the approval, `custom-tools list` and every run show the repo
  (top folder and origin URL, credentials removed), branch and HEAD commit, the last commit that changed the
  script, and its changes not committed (untracked, ignored, staged or not, with +/− lines since HEAD).
- It gets the same environment as a plugin (data folder, app root) with no secret settings.
- In a terminal it has the console as usual. In the window, an interactive one runs in a pseudo-console
  (`internal/pty`, Windows ConPTY) shown in its run as a terminal (xterm.js, `frontend/vendor/`): it
  works as in a console — `Read-Host` (hidden with `-AsSecureString`), `PromptForChoice`, `pause`,
  colors, progress. Click it and type; Ctrl+C goes to the script. A file dropped on the window while no
  question takes a path is typed into it. Once the script exits the terminal stays, shrunk to the lines
  used. A script that is not interactive (or where there is no ConPTY) has its output in the run as text
  and no input.

PowerShell: parameters come from the `param()` block through the PowerShell parser (types,
`[Parameter(Mandatory)]`, `HelpMessage`, `[ValidateSet]`, `[Alias]`, defaults) and comment-based help
(`.SYNOPSIS` as the summary, `.PARAMETER`). A script without `param()` takes positional args as they are.
It runs as `-Command "& '<path>' -Name 'value' …"` with values as quoted literals (so `$false` and arrays
pass, which `-File` cannot), `-NoProfile -ExecutionPolicy Bypass` (the approval stands in for the execution
policy), exiting with the script's exit code. Windows PowerShell on Windows, else `pwsh`; `pwsh` also for
`#Requires -PSEdition Core` or a script only `pwsh` can parse. Parameter sets and dynamic parameters are
not modelled (noted in `--help`).

## Tests

`go test ./...`

## Config

Lowest to highest priority:

- `.env` (repo / next to the exe): shared non-secret config (base URLs), committed (built into the exe).
- Real environment variables.
- App settings, `.env.config` (data folder): where auth settings belong. Written by `configure` and by
  missing-setting prompts. Overrides everything, including real environment variables.

App settings with defaults, written to `.env.config` on first start (keeping a valid value already set in a
fallback source). Invalid values fall back to the default with a warning. Read live, so `configure` changes
apply in the open window:

- `HOURS_PER_DAY` (default `8`) — working hours per day for the quota.
- `TZ_OFFSET_HOURS` (default `7`) — UTC offset all dates are computed in (ranges, "today", worklog days,
  file timestamps); quarter hours allowed, e.g. `5.5` = UTC+5:30.

The window's header shows the current values, the settings file path, the credential store in use and the data folder.

## Credentials

`internal/secrets` stores the AEXT session and `JIRA_TOKEN` (settings with `Credential` set in
`internal/settings/env.go`) behind one `Store` interface, in the OS credential store:

- Windows: Credential Manager (generic credentials `aex:<data folder>:aext-session` / `:jira-token`)
- macOS: Keychain
- Linux: Secret Service over D-Bus (GNOME Keyring, KWallet)

Where none is available (e.g. headless Linux without a keyring daemon), or with `AEX_CREDENTIAL_STORE=file`,
they go to `credentials.json` in the data folder (0600). Entries are scoped to the data folder, so
`--data-dir` keeps separate logins. `account` shows which store is in use.

Saving `JIRA_TOKEN` (configure, `account`, missing-setting prompt) writes the credential store and removes
any `JIRA_TOKEN` line from `.env.config`. A token still in `.env.config` from older versions keeps working
until saved again. An old `session.txt` is ignored: log in to AEXT again.

`.claude/settings.json` denies Claude Code access to `.env.config`, `session.txt` and `credentials.json`.
