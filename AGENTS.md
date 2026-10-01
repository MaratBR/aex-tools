# AGENTS.md

How aex works inside, for coding agents (and anyone changing the code). [README.md](README.md) is for
people using aex: keep it short, and put the details here. Building, dev builds and tests are in
[BUILD.md](BUILD.md).

## Working here

- Build and test as in [BUILD.md](BUILD.md): `go vet ./...`, `go test ./...`; `gofmt` every Go file.
- Files use LF line endings (write them that way; tools on Windows may add CRLF).
- A change people notice (a tool, flag, widget, setting) gets a line in README.md, and its details here.
- Code comments and docs say what things do, in plain words; match the style around them.

## Layout

- `main.go` — entry point: global args, tool registry, dispatch; embeds `.env`. No tool opens the window.
- `gui.go` — opens the window (`internal/webui`) over the tools; `gui_windows.go` closes the console
  Windows gives the exe when it is started from the Start menu or Explorer, and tells whether this is
  the window exe (see Windows exes)
- `header.go` — the window's header: who is logged in to AEXT (`/api/auth/me`), Jira
  (`/rest/api/3/myself`) and Google (refreshing its access token), checked in parallel and never prompting, then settings and the data folder;
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
    `--help`); `adapter/powershell` — the PowerShell one, `adapter/autohotkey` — the AutoHotkey v2 one
  - `gitinfo` — a file's git repo, commit, last commit that changed it and its changes not committed;
    a repo's state (branch, changes, commits to push and pull) for the Git status widget
  - `gitclient` — the git client installed on the device (Fork, GitHub Desktop, …): finds it, opens it,
    reads its icon
  - `proc` — starting console programs without a console window (from the window)
  - `pty` — running a console program in a pseudo-console (ConPTY) the window shows as a terminal
  - `settings` — app root (repo or exe folder), data folder (`--data-dir`), output paths; loads `.env`,
    app settings (`.env.config`), the credential store; auth settings list, prompts for missing ones and saves
    them to app settings; `HoursPerDay()` / `TZOffsetHours()`, Jira→CSV project map, calendar country,
    leave statuses (`config.go`)
  - `shortcut` — adds aex to the Start menu (Windows), `~/Applications` (macOS) or the app menu (Linux)
    and removes it; asks once on first launch
  - `autostart` — starts aex when you log in (Windows `Run` registry key, macOS LaunchAgent, Linux XDG
    autostart entry) on the days picked; `--autostart` exits at once on other days
  - `dates` — day math and range expressions
  - `jira`, `aext` — API clients
  - `google` — Google sign-in (see Google) and the Calendar API client
  - `httpx` — on any unexpected response (status, non-JSON, wrong shape) dumps status, URL,
    relevant headers (cookie values redacted) and body (capped at 300 KB) to stderr
  - `reminder` — reminders on top of all windows on every screen, with a chime (see Reminders)
  - `browser` — opens web links, in the browser an `aex+<browser>://` link names (see Reminders)
  - `quota` — monthly quota from AEXT working days, leaves and logged hours
  - `ui` — ANSI colors (only on a terminal; `NO_COLOR=1` disables, `FORCE_COLOR=1` forces), line
    prompts, hidden input, key press; `ui.Remote` answers all prompts elsewhere (the window)
  - `webui` — the window ([Wails](https://wails.io), the system WebView2 on Windows): see Window
  - `about` — version, build (time, commit) and licenses for Settings > About: see About and licenses
- `plugins/<name>` — plugin sources (`main` packages), built to `plugins\<name>.exe` next to the exe
- `bin\<name>.ps1` — PowerShell entrypoint per tool (thin wrapper over `bin\_invoke.ps1`)
- `bin\build-exe.ps1` — builds the release `dist\aex.exe` and `dist\aex-cli.exe`; `bin\_plugin-hashes.ps1` — the hashes of the
  plugins just built, for aex's pre-approved hashes (see Plugins); `bin\dist.ps1` — vets, tests, builds
  the exes and the installer, and zips the installer with `SHA256SUMS` to `dist\aex-<version>-windows-<arch>.zip`;
  `bin\dist-install.ps1` — runs `dist.ps1`, unzips its zip to `dist\install\` and runs the installer
- `installer\aex.iss` — the Windows installer (Inno Setup 6): per user, no admin, into
  `%LOCALAPPDATA%\Programs\aex`; adds the same `aex.lnk` to the Start menu as `internal/shortcut`
  (so aex does not offer it), optionally a desktop shortcut; uninstalling removes the files, the
  shortcuts and the autostart `Run` value, and keeps the data folder
- `assets\logo.svg` — icon source; `assets\logo.ico` rendered from it (16–256 px)
- `winres\winres.json`, `rsrc_windows_*.syso` — exe icon and version info

## Windows exes

On Windows aex is two builds of the same program, side by side (`settings.WindowExeName`,
`CLIExeName`):

- `aex.exe` is built with `-H windowsgui`, so Windows starts it with no console: it opens the window
  and nothing flashes. Given a tool (or `--help`), it says to use `aex-cli.exe` (on the console of the
  terminal it was started from, else in a message box) and exits with 2 (`pointToCLI`). It tells it is
  the window exe from its own PE header (`isWindowExe`).
- `aex-cli.exe` is a console program: tools in the terminal, the window without a tool. The dev
  scripts build only this one (`dist\dev\aex-cli.exe`).
- Shortcuts and starting on login point at `aex.exe` next to the running exe when there is one
  (`settings.WindowExe`), else at the running exe.
- The window has no console, so every console program aex starts from it must not get one of its own
  shown: output read through pipes uses `proc.HideConsole`; a plugin run uses `proc.HideConsoleIfNone`
  (from a terminal it shares that terminal).

macOS and Linux have one `aex`.

## Data folder

Per-user app data, by default `%APPDATA%\aex` (macOS: `~/Library/Application Support/aex`,
Linux: `$XDG_CONFIG_HOME/aex` or `~/.config/aex`). Holds:

- `.env.config` — app settings: auth settings saved by `configure` or prompts
- `credentials.json` — AEXT session and `JIRA_TOKEN`, only when there is no OS credential store (see Credentials)
- `output\jira-export\` — generated files
- `custom-tools.json` — the custom tools added (name, script path, adapter)
- `update-check.json` — the last check of how far the build is behind GitHub (see About and licenses)
- `reminders.json` — the reminders set up in Settings > Reminders, each with when it last showed
- `home.json` — the window's home page: its widgets with their places, sizes and settings, and
  whether auto refresh is off
- `themes\*.json` — custom themes for the window (see Themes)

Change it with `--data-dir <dir>` (accepted before or after the tool name) or the `AEX_DATA_DIR`
environment variable (real environment only). Click its path in the window's sidebar to open it.

## worklog-sync

Exports Jira worklogs to `<data folder>\output\jira-export\<timestamp>.csv` (`date,project,hours,description`,
one row per day + issue), then offers to send it to AEXT.

Before sending, rows AEXT already has (`GET /api/time-entries/`: same date, project and description) are
skipped and listed; so are rows repeated in the CSV (the first row counts). When the hours differ, the
AEXT entry is changed to the CSV's (`PATCH /api/time-entries/<id>` with `{"hours_total": …}`) and both
hours are shown; when AEXT has several entries for the row, which one to change is unclear, so they are
left and both hours are shown.

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
expected hours (`HOURS_PER_DAY` app setting), logged, % filled, hours behind as of today (today included:
nothing logged today is a full day behind), and hours/day needed over the remaining working days (today
included).

Leaves (`/api/leaves/my-requests`, fetched per calendar year) remove their working days from the quota
and from gap detection in `worklog-sync`. Hours logged on leave days, and on days the calendar has as not working
(weekends, holidays), are warned about under the month ("Off time"). `declined` and `cancelled` leaves are ignored with a warning
(`IgnoredLeaveStatuses` in `internal/settings/config.go`); `pending` (or any other non-`approved` status) is counted, with a warning;
a leave created by someone other than you (by email) is also reported. Leave records are mostly
redacted, so only ids, emails, dates, type and status are used.

## account

`.\bin\account.ps1` shows who you are logged in as: AEXT (`/api/auth/me`: name, email, other profile fields,
server, session file), Jira (`/rest/api/3/myself`: name, email, account id, time zone, …) and Google (email,
whether Calendar access was granted), plus the login settings and where each comes from (secrets masked). On
a terminal it then offers actions until Done:

- log in to AEXT (new emailed code, replaces the cached session)
- log out of AEXT: `POST /api/auth/logout` (expects 204), then deletes the saved session
- wipe the AEXT session: deletes the saved session only, the server session stays valid until it expires
- set or change the Jira login: email + API token, checked against Jira before saving to app settings
- remove the Jira token from app settings
- log in to Google in the browser (see Google), replacing the saved login
- log out of Google: revokes the login at Google (`POST /revoke`), then deletes it

Logout, wipe, removing the token and replacing a login ask for confirmation (default no). Flags run one
action directly: `--show`, `--login aext|jira|google`, `--logout aext|jira|google`, `--wipe-session`.

## configure

In the window, settings are a page rather than a tool: Settings in the sidebar (or the Settings line under
the tools, or typing `configure`) opens it. Its menu has General, Widgets (the settings of widgets, by widget: the same form, saved the same way),
Reminders (see Reminders), About (see
About and licenses) and, under
Plugins, each plugin that has settings. General edits every setting below at once (checked before anything is saved; an empty field
removes the setting, or resets it to its default), picks the theme and mode (see Themes), toggles the app launcher entry, and
wipes after you type `CONFIRM`. A plugin's entry runs its settings (see Plugins) right there, its questions
asked on the page. The rest of this section is the terminal tool, `aex configure`.

`.\bin\configure.ps1` lists the settings with their current values: the auth settings `AEXT_EMAIL`,
`JIRA_EMAIL` and `JIRA_TOKEN` (hidden input), then `HOURS_PER_DAY`, `QUOTA_WARN_HOURS` and `TZ_OFFSET_HOURS` (validated). Pick one
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
- Wipe everything: all settings, the AEXT session, the Google login (not revoked), and every entry of the data folder (`output/`, plugin data,
  anything else). Plugins and their approvals are kept. Refused when the data folder holds the app.

`.env` and environment variables are never touched and still apply as fallbacks.

The `configure` list (or `configure --add-shortcut` / `--remove-shortcut`) also adds aex to the OS app launcher, pointing at
the running exe (with `--data-dir` if it was given), or removes it:
- Windows: `aex.lnk` in the Start menu (`%APPDATA%\Microsoft\Windows\Start Menu\Programs`), with the exe's icon.
- macOS: `~/Applications/aex.app`, a small bundle that opens the exe in Terminal. Only a bundle aex made is removed.
- Linux: `aex.desktop` in `$XDG_DATA_HOME/applications` (default `~/.local/share/applications`), run in a terminal.

The first time a release build opens the window, it asks once whether to add it (skipped when it is already
there); the answer is remembered as `SHORTCUT_ASKED` in app settings, so wiping settings asks again.

`configure --autostart <days>` (or the `configure` list, or Settings in the window) makes aex start when you log in, with
`--autostart` (and `--data-dir` if it was given): days are `mon,tue,wed,thu,fri,sat,sun` (comma separated), `workdays` or
`every-day`, saved as `AUTOSTART_DAYS` in app settings (not set: workdays). Started that way on a day not picked, aex exits
at once. `configure --autostart off` stops it.
- Windows: an `aex` value in `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.
- macOS: `~/Library/LaunchAgents/com.aexsoft.aex.plist`.
- Linux: `aex.desktop` in `$XDG_CONFIG_HOME/autostart` (default `~/.config/autostart`).

The window's onboarding (first start) asks too, set to workdays in release builds (off in dev builds).

Tools that need a missing auth setting prompt for it and offer to save it to app settings too.

## plugins

`.\bin\plugins.ps1` manages plugins (see Plugins). Without args on a terminal it lists them and asks
what to do; else `list` (default), `describe [<name>]` (asks to approve any not safe yet),
`allow <name>` (approves it and grants the access it asks for, without running it: what its widgets need),
`delete <name> [--yes]` (deletes the file and forgets its safe hash and granted access), `forget-all [--yes]` (forgets every plugin's safe hash and
granted access, keeping the files: each asks to be approved again on its next run), `open` (opens the
plugins folder). The credential store cannot list its keys, so approved plugin paths are also kept in
`plugin-approved-paths`; `forget-all` clears those plus the plugins in the folder now. Custom tools'
approvals are in that list too, so `forget-all` forgets them as well.

## custom-tools

`.\bin\aex.ps1 custom-tools` manages custom tools (see Custom tools). Without args on a terminal (or
from the window) it lists them and asks what to do: add a script (asks which adapter runs it, or to pick
it by the file; then its path, the file dialog offering that adapter's files, and a name), describe or
remove one, or view the adapters not supported here, with why (shown only when there are some). Else `list` (default: file, SHA-256, state, adapter, parameters, git info), `describe <name>`
(that plus its `--help`), `add <path> [--name <name>] [--adapter <adapter>] [--approve]` (named after the
file by default, e.g. `Deploy App.ps1` → `Deploy-App`; run by the first adapter that handles the file
unless `--adapter`, which must handle it too), `remove <name> [--yes]` (forgets its safe hash; the script
is kept).

After adding, the tool's info is shown (path, SHA-256, adapter, parameters, git) and, on a terminal, it
asks whether to approve it now (default no): yes saves that SHA-256 as its safe hash
(`custom.Approve`, refused if the file changed since it was shown), so its first run does not ask.
`--approve` does so without asking; otherwise it asks to be approved on its first run.

## cm-release (plugin)

A group of CM release tools (`.\bin\cm-release.ps1 <tool> [args]`). Needs Jira access (see Plugins).

The git tools work on every CM repo at once: `<reposDir>\<repo>` for each of the `repos` settings
(default `clearmechanic.frontend`, `clearmechanic.siteforappointments`, `cmos.datamigration`, `src`,
`cmos.microservices`). The repos folder is asked for on first run; both are in the plugin's settings (the
window's Settings page, or `jira-handoff --settings`). Branches: `master`, `PROD`, `staging`, `release/VERSION`; tag `vVERSION`.
The plugin also gives these repos to aex (`plugin.GitRepos`) as the Git status widget's default list.

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
- Settings (the plugin's settings on the window's Settings page, or `--settings`): default
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

## Window

`aex` without a tool opens the window: the pages (Home, Runs) and the tools on the left (a group opens
under its name), the logins and settings under them, and the page on the right. It opens on Home when
Home has widgets, else on Runs. Running a tool, and any question asked, shows Runs; a dot on Runs says
a run printed while Home was shown. Tools are the same console programs as in the terminal:

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
Colors and fonts are in `tokens.css`, shared by the window and the widgets.

### Themes

The window's look is a mode (System, Light or Dark) and a theme: its colors (and fonts), the
variables of `tokens.css` without their `--`, for light mode, dark mode or both. A theme with only one
of them uses it whatever the mode; a variable it leaves out keeps the default's. Settings > General >
Appearance picks both (`themeRows` in `settings.js`), each theme a card drawn in its own colors.

- `themes.go`: `App.Themes` lists the default theme first (its variables read from `tokens.css`, only
  to draw its card), the built-in ones (`internal/webui/themes/*.json`, embedded: GitHub, Solarized,
  Nord, Gruvbox, Catppuccin with both modes; Dracula, Tokyo Night, Monokai dark only), then the custom
  ones, `<data folder>\themes\*.json` (id `custom:<file>`), each by name; a file that is not a theme is
  listed under the cards with why. A file is `{"name", "light": {…}, "dark": {…}}` (64 KB at most; the
  name defaults to the file's): only the variables in `themeVars` (colors, `c0`–`c15`, shadows, fonts;
  not `ring`), each value without `;{}<>\!@`, `/*`, `url(` and the like, since it goes into widgets'
  pages too. `App.CopyTheme` copies a theme into the folder (`<name>-custom.json`, then `-2`, …) for
  Customize, which picks the copy and opens the folder; `App.OpenThemes` opens it. Reload reads them
  again.
- `theme.js` (in `<head>`) keeps the mode and the theme picked, with its colors, in the web view's
  storage, so the page starts in them; `theme.refresh()` (at start and on the Settings page) takes the
  theme again from `Themes` (its file may have changed) or goes back to the default when it is gone.
  It sets `data-theme` on `<html>` to light or dark (what `style.css` and widgets key on) and the
  theme's variables as inline styles, then fires `aex:theme` on `window`: terminals (`term.js`) and
  widgets (`home.js`) follow it.
- Widgets get the scheme and variables in their page (`widgetDoc`) and in each `theme` message
  (`{theme, vars}`, set by `sdk.js`).

### Home and widgets

Home takes the whole window: the sidebar slides away there, and the button at the top left (or Ctrl+B)
brings it back or hides it again, remembered in the web view's storage; a dot on the button says a run
printed meanwhile. Runs always shows the sidebar.

Home is a grid of widgets (12 columns, 6 or 1 when the window is narrow, a widget taking the whole row
when it is wider than that; rows 150 px). Each widget has a place at full width (`HomeWidget.X`, `Y`:
the column and row of its top left corner), anywhere, gaps allowed; in a narrow window the widgets
follow one another in that order (top to bottom, then left to right). Add widget picks one (a widget
can be added more than once) and puts it in the first free place it fits, from the top left.
Edit shows the grid's cells, with room below the lowest widget, and puts a cover on each widget to drag
it to any cell (in a narrow window: onto another widget, to trade places), move it a cell at a time
with the arrows, change its width (1–12 columns, up to the right edge) and height (1–4 rows), with the
steppers or by dragging its right edge, bottom edge or corner, or remove it (its place stays empty). A
widget moved or grown onto others pushes them down to the first row where they fit (`settle` in
home.js); while dragging, they come back as it goes on, and letting go outside the grid (or Esc) puts
everything back. Every change is saved to `home.json` in the data folder (`App.SaveHome`; sizes and
places are clamped on load, overlapping widgets moved down: `placeWidgets`; plugins' widgets stay, see
below). A file from before places (no `x`, `y`) gets them as the grid packed it then. A placement of a widget aex does not have
(removed, renamed without `renamedWidgets`, or from a newer aex) stays too, and shows a warning in its
cell: unknown widget, with its id, to remove in Edit (`WidgetPage.Unknown`); only ids that are not a
widget id at all are dropped. With `--debug`, Add widget also offers "Unknown widget"
(`unknownDebugWidget`): a made-up id (`debug-unknown-…`) and settings, new each time, to see how one
looks. A
`home.json` from the 4-column grid (no `"columns": 12`) has its widths multiplied by 3 on load. A
plugin's widget sizes (`plugin.Widget.W`) are still in quarters of the width, multiplied by 3 too.

Auto refresh: a widget can ask to be refreshed on its own every so many seconds (`WidgetInfo.Refresh`
in `widgetCatalog`, `plugin.Widget.Refresh` for a plugin's, at least 5 s; none by default). The Auto
refresh switch on Home (shown while such a widget is on it) turns it off for all of them, saved as
`autoRefreshOff` in `home.json`; it is on by default. Refreshes pause while Home is not shown or the
window is minimised (`WindowIsMinimised`, since the web view keeps running then); a widget that fell
due meanwhile refreshes as soon as Home is back.

A widget is one page (its HTML, CSS and JS), from the app (`frontend/widgets/<id>.html`) or from a
plugin (see Plugins). Each runs in a sandboxed frame (`sandbox="allow-scripts"`, an origin of its own):
it cannot reach the window, its backend or other widgets. The window puts at the top of its `<head>` a
Content-Security-Policy with no network (inline scripts and styles, `data:` images and fonts), the
window's colors (`tokens.css`), `widgets/widget.css` (base styles, buttons) and `widgets/sdk.js`; the
theme follows the window's. `sdk.js` gives it `aex`, which works through messages to the window:

- `aex.call(name, args)` — a data call. Built-in widgets call `widgetAPIs` (`internal/webui/home.go`);
  a plugin's widget only its plugin's calls. They never prompt, since a widget has no run to ask in:
  AEXT ones use `aext.NewQuiet`, which fails with `aext.ErrNoSession` instead of logging in.
- `aex.run(line, {home})` — runs a tool on Runs, as typed on the command line; a plugin's widget only its
  plugin's tools. With `home: true` the window goes back to Home once the run succeeds, if Runs is
  still shown then (a failed run stays on Runs).
- `aex.settings` / `aex.saveSettings(obj)` — this placement's own settings: a JSON object (8 KB at
  most) kept with it in `home.json` (`HomeWidget.Settings`), `{}` when it is added, such as which
  calendar it shows. home.js puts them in the page as it loads it.
- `aex.remind({title, message, urgent})` — shows a reminder (see Reminders; `urgent`: extra urgent), a
  Promise; `App.Remind`.
- `aex.onRefresh(fn)` — `fn` runs after every run finishes (it may have changed the data; for a
  plugin's widget at most every 30 s, since each call starts the plugin) and on auto refresh. Widgets
  that could not load try again then.
- `aex.openPopup({title, width, data})` — opens the widget's page again in a popup over the window
  (below the title bar), for what does not fit in its cell, such as its settings: the same page in a
  sandboxed frame of its own, with `aex.popup` = `{data}` (null in the cell), the same calls, tools
  and settings. It is as tall as the page (sdk.js reports its height), up to the window's, then it
  scrolls; `width` in px (480 by default). A Promise of the value given to `aex.closePopup(value)` in
  it, or undefined when closed otherwise: ×, Esc (unless the page's own handler called
  `preventDefault`), a click outside, leaving Home, the widget removed or its page leaving. One popup
  at a time (a second rejects). Settings saved in one page reach the other (`aex.onSettings(fn)`,
  `aex.settings` already updated); refreshes and the theme reach both. `openPopup` in home.js.

A widget that leaves its page (a link, `location`) is replaced by a note. The sandbox protects the
window from a widget; blocking the network is only a second line, since an approved plugin is trusted
like any program.

Adding a built-in one: write `frontend/widgets/foo.html`, add it to `widgetCatalog` in `home.go`
(name, summary, the size it is added with, how often it refreshes), and any API it needs to `widgetAPIs`.

Widgets so far:

- `quota` (refreshes every minute) — AEXT quota: this month's hours against the quota (the quota tool's numbers: behind or
  ahead, due by today, hours/day to finish, working days without hours with a button to run
  worklog-sync), a button to sync this week (`worklog-sync --range "this week"`: Monday through today)
  and whether last month is complete. ‹ › show other months (up to 12 ahead; the `quota` call takes
  `{"month": "YYYY-MM"}`), kept as months from this one, so it follows when the month turns; This
  month goes back. A past month shows complete or short, its missing days with a button to sync
  them (`--range` from the first to the last), and the month before it; a future one is Upcoming.
  The bar has a slot per weekday (`Month.Days`): working days fill with the hours logged, in order,
  a day's hours each, and leave (approved green, pending blue, hatched) and holidays (grey) are
  skipped; an Off row lists them as periods. Hours logged on leave days and on weekends and
  holidays are a warning ("You worked 12h during your off time and 3h on non-working days",
  `quota.OffWork`). A month over and short by less than `QUOTA_WARN_HOURS` (default 1) shows in the
  warning color (yellow) instead of red, here and in the quota tool. While it loads (the first time, or
  a month not loaded yet) grey blocks stand where the numbers go. One row high it shows only the
  numbers and the bar.
  Without an AEXT session it offers to log in (`account --login aext`).
  With `--debug` the `quota` call also returns what the numbers came from (`QuotaDebug` in `home.go`:
  now, the device's clock, the settings, AEXT's raw working days, summary and leaves with how long each
  took, `quota.Raw`, and the day maps made of them), and a bug button opens the month shown in a popup
  with it all: both months' fields, every day (calendar, kind, leave, counted, hours, flags), the
  leaves (ignored ones struck through), warnings and the raw reply.
- `google-calendars` (every minute) — Google Calendar: the events on now in one calendar, each with the
  time left and (smaller) when it ends (an all-day one: "all day", or "last day" on the last of several), then the upcoming ones through the third working day after today
  (AEXT working-days calendar with a session, else Monday to Friday), by day, one line each: start time,
  name and length (time to the start instead when within 15 minutes, outlined). Hovering an event shows
  its full times. Events are
  tinted in the calendar's color; cancelled ones and ones you declined are left out; free and tentative
  ones are marked. The first time it lists the account's calendars to pick one, kept in its placement's
  settings, so each copy can show another; the calendar button changes it. The filter button sets a
  regular expression over event titles (ignoring case), also per placement, to show only the events
  that match or hide them; the button is tinted while one is on. It counts down between
  refreshes. Without a Google login it offers to log in (`account --login google`).
- `jira-tickets` (every minute) — Jira tickets (`jirawidget.go`), each with its key, summary, type,
  priority, when it was updated and its status; clicking one opens it in the browser. When added it
  asks which, and how many at most (20 by default, up to 100), kept in its placement's settings (the
  filter button changes it):
  - Assigned to me: open tickets (status not Done, most recently updated first) with
    `assignee = currentUser()`. Tickets assigned to you in the last 24 hours (72 on a Monday, in the
    configured TZ, to cover the weekend) are marked New, tinted and listed first, with a warning above
    the list: `assignee CHANGED TO currentUser() AFTER "-24h"`, or created since then.
  - Where I'm in a field: you pick custom fields of people (`/rest/api/3/field`, schema `user` or an
    array of `user`), and it shows tickets with you in any of them (`cf[<id>] = currentUser()`, which
    matches a field of several people too); open ones, most recently updated first.
  - JQL: whatever a query you type matches, in its order. Jira checks it before it is kept; when a
    search fails, what Jira finds wrong with the query (`/rest/api/3/jql/parse`) is shown.

  In every list, a ticket in a To Do status (status category `new`) not updated for over 2 weeks is
  muted (faded), unless it is marked New.

  The + button adds a tab: the widget then shows tabs in place of its title, each a list of its own
  (any of the above, with its own name and how many at most) with its count; the filter button changes
  the tab shown or removes it, and with one tab left the tabs go away. All lists load at once.

  Without a Jira login, or when Jira rejects the token, it offers `account --login jira`. One row high
  it shows only the count and the warning.
- `cat` — Cat as a service: a random cat from `https://cataas.com/cat` (`cat.go`, fetched by aex and
  handed to the widget as a `data:` URL, since widgets have no network), filling the whole widget with
  no padding (cropped to fit). Clicking it brings another: 4 cats are fetched (two at a time) and
  decoded ahead, so it shows at once, coming in from a little larger and blurred over the one before
  (a plain swap with reduced motion). No auto refresh. On hover, 5 stars over the
  bottom of the picture rate the cat; the rating does nothing (not kept or sent anywhere), and each of
  the 5 ratings has its own message saying so. Each cat is a he or a she at random, for the messages.
- `2048` — the game of 2048 on a 4×4 board: arrow keys or WASD once the widget has the focus (click
  it), or a swipe. Reaching 2048 offers to keep going. The board, score and best score are kept in the
  placement's settings, so a game goes on after the window closes. No auto refresh.
- `clock` — the time now in one or two time zones (IANA names; `America/Chicago`, Central Time, by
  default), each with its place, short zone name (CDT, GMT+5) and how far it is from the device, and
  its date, marked Tomorrow / Yesterday (or the date) when it is not today on the device. 24-hour by
  default, or AM/PM. The clock button (on hover) picks the zones, from every zone the web view knows
  (`Intl.supportedValuesOf`), and the format, kept in the placement's settings (`zones`, `h24`). Two
  clocks sit side by side or one under the other, whichever lets the time be larger, and the time
  is sized to fill its cell (`fit`, measured on every resize). It redraws
  itself every minute; no auto refresh.
- `git-status` (every 5 s) — Git status (`gitstatus.go`): the repos this placement watches, each with
  its branch and uncommitted changes, commits to push (↑) and to pull (↓, as of the last fetch), with
  Clean or Pending changes (uncommitted changes, unpushed commits or a git error in any repo). No
  fetch, so it loads quickly. When added it asks for the repos (their folders, typed, pasted or picked
  with "Choose folder…", the system dialog: `chooseFolder`, one of `windowAPIs`), starting with those
  of the first approved plugin that provides some (`plugin.GitRepos`, see Plugins: cm-release gives
  its CM repos); kept in the placement's settings (`repos`), the folder button changes them. At the
  bottom right, a button with the icon of the git client found on the device (`internal/gitclient`:
  Fork, GitHub Desktop, GitKraken, Sourcetree, Sublime Merge, SmartGit or TortoiseGit, looked for in
  that order; the icon read from its exe, or its app on macOS) opens it; hidden when none is
  installed. One row high it shows only the verdict. It was the cm-release plugin's
  `cm-release/cm-repos-state`: such a placement in `home.json` loads as this one
  (`renamedWidgets`), asking for its repos.
- `shortcuts` — Shortcuts: buttons that run tools (`aex.run`), each a rounded tile in its color
  (picked, transparent for no tile, or one by its place) with a built-in icon, an emoji or, with neither, its name's initials,
  and its name under it. A button has a name, an icon or both (at least one), the tool it runs,
  picked from every tool that runs on its own (`tools`, one of `windowAPIs` in `shortcuts.go`: a
  group's tool as `<group> <tool>`), and arguments added to the line. Once its tool succeeds the
  window goes back to Home, unless the button is set to stay on Runs. Kept in the placement's
  settings (`name`: an optional title, `buttons`: `name`, `icon`, `color`, `tool`, `args`, `stay`;
  24 at most); the pencil button (on hover) edits them, in a popup (`aex.openPopup`), since the widget is
  often too small for it. One button fills the widget; several fill it as a grid (scrolling when they
  do not fit), under the title when one is set. A button whose tool aex no longer has
  is greyed out. No auto refresh; the tool list is read again after every run.

## Reminders

A reminder (`internal/reminder`) is a message, with an optional title ("Reminder" when none; 100 and
1000 characters at most), shown on top of all windows on every screen with a soft chime until it is
dismissed.

- Windows: a card per monitor (a topmost Win32 window drawn with GDI, per-monitor DPI), centred near
  the top of its work area, with the title, the time it appeared, a close button (×, top right) and
  the message, wrapped word by word (`layout`), its links underlined. × on any screen closes it on
  all; a link opens and leaves it open, unless marked to close it (below). It never takes the focus (`WS_EX_NOACTIVATE`), is not on the taskbar,
  and follows Windows' light or dark app mode. Each reminder runs its windows on a thread of its own;
  several at once are cascaded. The chime is two bell notes made in code (`chime.go`, a WAV played
  with `PlaySound`).
- Extra urgent (`Reminder.Urgent`): the card's border, dot and title are red, and it chimes twice
  (2 s apart) when it shows, then twice again every 30 s until it is closed (cutting the chime
  short) or 10 minutes pass; the card stays after that (`announce` in `chime.go`).
- macOS and Linux: a system notification with a sound (`osascript`, `notify-send`); links show as
  written, and an urgent one chimes once.

Links in the message (`Reminder.Parts`): `[label](link)` or a bare link, each `http(s)://…` or
`aex+<browser>://<address>`; anything else (`file:`, `javascript:`, …) stays text. `!` right before
the link (`CloseMark`: `[label](!link)`, or a bare `!link` at the start or after a space, so
`Hi!https://…` is a plain link) makes it close the reminder once it opens (`Part.Close`). `browser.Open`
opens an `aex+` one in that browser, the address as https unless it has a scheme of its own
(`aex+firefox://http://intranet`), and in the default browser when that one is not installed (or
not known: then a command of that name is tried). Browsers (`browser.Browsers`): chrome, edge,
firefox, brave, vivaldi, yandex, opera, opera-gx, chromium, librewolf, waterfox, arc, safari (and a
few aliases). Found on Windows through the browsers Windows has registered
(`SOFTWARE\Clients\StartMenuInternet`, user and device, shorter key names first, so Chrome comes
before its Canary), then App Paths, the folders it installs to and PATH; on macOS with `open -a`, on
Linux its commands on PATH.

Triggering one:

- A tool or plugin calls `reminder.Show`. In the window's process `AEX_REMINDERS=window` is set (tools
  and plugins inherit it), so `Show` prints a mark, `\x1b]aex-remind;<base64 of {"title","message"}>\x07`,
  which the window's output stream takes out of the run (`stream.mark`, `webui/reminder.go`) and shows;
  it stays after the tool exits. Any program whose output the window reads through the pipe can print
  that mark (not a custom tool shown as a terminal, nor a widget call). In a terminal `Show` shows it
  itself and waits until it is dismissed, since it goes with the process.
- A widget calls `aex.remind` (see Home and widgets).
- `aex --debug debug reminder [--title T] [--urgent] [--in 10s] [message]` shows one (`internal/tools/remind`).
- Settings > Reminders (`frontend/reminders.js`, `App.Reminders` / `SaveReminders`) sets up
  reminders that show at a time (HH:MM, in `TZ_OFFSET_HOURS`) on the days picked (`reminder.Scheduled`):
  title, message (with links), time, days, extra urgent, on or off; Show now previews one. Kept in `reminders.json`
  in the data folder (100 at most), each with the day it last showed (`last`), so it shows once a day.
  The window runs `reminder.RunSchedule` while it is open: every 10 s it reads the file and shows
  those due, up to 30 minutes late (aex closed or the device asleep at the time). Saving a new one, or
  one whose time or days changed, whose time today has passed already, does not show it today.

## Google

aex signs in to Google as an installed app: `account --login google` (or a tool that needs Google while
nobody is logged in) opens Google's sign-in page in the browser, which Google sends back to
`http://127.0.0.1:<port>/`, served by aex only for that login (PKCE and a state tie the answer to it).
Scopes: `openid email` (the account's email, from the ID token) and `calendar.readonly`. The login (refresh
token, email, scopes granted, the client it was granted to) is kept in the credential store
(`aex:<data folder>:google-login`); access tokens only in memory. A login Google rejects (revoked, or
expired: after 7 days while the Google Cloud app is in Testing) is deleted, and the next login asks again.
Widgets and the header never log in.

The OAuth client is the `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` settings, built in through `.env`. To use
your own, create a Desktop app OAuth client in a Google Cloud project with the Calendar API enabled and set
both in app settings (the secret goes to the credential store). A login made with another client is not used.

## About and licenses

aex is Apache-2.0 (`LICENSE`, `NOTICE` at the root). Settings > About (`frontend/about.js`,
`App.About` in `internal/webui/about.go`) shows:

- Build (`about.Info`): version (`ProductVersion` in `winres/winres.json`), build time
  (`-X aex/internal/about.builtAt=<RFC 3339>`, set by `build-exe.ps1`; else the exe file's modified
  time, marked so), the git commit with a link to it on GitHub (`about.Repo`), its time and whether the
  checkout had changes not committed (the `vcs.*` settings Go stamps into the exe; none with `go run`),
  Go version, platform.
- Updates (`about.CachedBehind`, `App.Behind`): how many commits the build's commit is behind `master`
  on GitHub (`GET api.github.com/repos/…/compare/<commit>...master`, no login), with a link to the
  comparison; also the last row of the sidebar (`#behind`, click opens About), not shown for a build
  without a commit. The answer is kept in `update-check.json` in the data folder for 6 hours (a failed
  check for 1 hour: not pushed, offline, GitHub's limit) and asked again only after that, or for another
  commit; Check now asks again unless the last check is under a minute old. The window reads it at start
  and after every run.
- Pre-approved plugins (`plugin.PreApprovedList`): each hash built in (see Plugins), with the file in
  the plugins folder that has it now (hashed, not run).
- aex's license and NOTICE, and every third-party component with its version, license (SPDX) and
  license files.

The licenses are in `internal/about/about.json` (embedded), written by `go generate ./internal/about`
(`internal/about/gen`) from the source:

- Go modules: the packages aex and its plugins link on Windows, macOS and Linux (`go list -deps`, tags
  `desktop,production`), with the license files (`LICENSE`, `COPYING`, `NOTICE`, `PATENTS`, …) in each
  package's folder and those above it up to its module's, so a nested license (e.g. go-webview2's
  `webviewloader`) is included only when that package is linked. The Go standard library from `GOROOT`.
- Vendored files (`vendored` in the generator; every file in `frontend/vendor` must be listed), and
  binaries a package builds in whose license is elsewhere (`embedded`: WebView2Loader.dll, its license
  in `internal/about/notices/`).
- A module without a license file is refused unless listed in `noLicenseFile` with what it states.

Each license is named from its text (an SPDX identifier line, or the known wording of Apache-2.0, MIT,
ISC, BSD-2/3-Clause, MPL-2.0, Unlicense); an unknown one fails the generator. So does one not in
`allowed` (permissive licenses and MPL-2.0; no GPL, LGPL or AGPL), since aex could not ship under
Apache-2.0 with it. `TestAboutJSONUpToDate` runs `gen -check` and fails while `about.json` differs
from what the source gives.

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
  `plugin.Main(tool.Tool{Name: "foo", Summary: "...", Run: run})` (after the tool: its access and
  widgets). The build scripts build every
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
- Pre-approved hashes: the plugins built from this repo (`plugins/<name>`) are safe without asking. The
  build scripts build them first, hash each (`bin\_plugin-hashes.ps1`: only files named after a folder in
  `plugins/`, nothing else in the plugins folder) and build aex with those hashes
  (`-ldflags "-X aex/internal/plugin.preApproved=<hash>,<hash>"`, `internal/plugin/preapproved.go`). Any
  plugin file with one of them is safe, wherever it is; `plugins list` shows it as "safe (built with aex)".
  Rebuilding a plugin without rebuilding aex makes it ask again. Custom tools are never pre-approved.
  Access is still asked for as usual.
- Settings: a plugin with settings of its own says so with `plugin.Main(t, plugin.Settings{Summary: "...",
  Run: run})` (`"settings": "<summary>"` in the describe JSON). The window lists it on its Settings page,
  under Plugins, and runs it there as `<plugin> --aex-settings` (approval and access as for any run), its
  questions answered on that page. Run asks for the settings and saves them where the plugin keeps them
  (`settings.PluginSettingsDir`, which wiping settings deletes).
- Widgets: a plugin can offer widgets for Home (see Window), each with its page built into the plugin
  (so its approval covers the page too) and its data calls: `plugin.Main(t, plugin.Jira,
  plugin.Widget{ID: "foo", Name: "...", Summary: "...", W: 2, H: 1, Refresh: time.Minute, HTML: page, Calls:
  map[string]func(json.RawMessage) (any, error){...}})`, `page` being a `go:embed`ded file. They are
  listed in `--aex-describe` (`"widgets": [{"id", "name", "summary", "w", "h", "refresh", "access"}]`, refresh in seconds) and are `<plugin>/<id>`
  in the window. aex gets the page with `<plugin> --aex-widget <id>` (cached per file hash) and runs a
  call with `<plugin> --aex-widget-call <id> <call>`: args as JSON on stdin, `{"result": ...}` or
  `{"error": "..."}` on stdout, 20 s at most, its output never shown in a run. Neither asks anything:
  the plugin must be safe and, for calls of a widget needing access, have it granted and its settings
  set; else the widget says what is missing, with Review (runs `plugins allow <name>`). A widget needs
  the access in `plugin.Widget.Access`, some of the plugin's (`Access: []plugin.Access{plugin.Jira}`):
  one needing none works while the plugin's access is not granted, and its calls get no credentials.
  A plugin built before widgets listed their access lists none, and its widgets need all the plugin's.
  A call gets the plugin's settings and the secrets of the widget's access, as a run does. Plugins not approved cannot describe themselves, so Add widget lists
  them under "Don't see the widget you need?", each with Review.
- Provides: a plugin can give aex data it asks for, with `plugin.Main(t, plugin.Provide{Name:
  plugin.GitRepos, Run: run})` (`"provides": ["git-repos"]` in the describe JSON). aex gets it with
  `<plugin> --aex-provide <name>`: `{"result": ...}` or `{"error": "..."}` on stdout, only while the
  plugin is safe, never asking anything, with its settings and no access (`plugin.Provided`). So far:
  `git-repos`, the folders of the git repos it works on, the Git status widget's default list.

## Custom tools

A custom tool is a script anywhere on disk, added with `custom-tools add <path>`, run by the tool
adapter for its kind of file. Custom tools are grouped by adapter: a group named after it (`powershell`,
`autohotkey`, in the order of `allAdapters` in `internal/custom`, only with tools in it) shows in the window after the
plugins, and a tool runs as `aex <adapter> <name> [args]` (or `aex <name> [args]`, found in the groups
when no tool has that name). No custom tool can be named like an adapter; a group named like a
built-in tool or a plugin is skipped with a warning.
Adapters so far: `powershell` (`.ps1`), `autohotkey` (`.ahk`, `.ah2`: AutoHotkey v2). A new one implements `adapter.Adapter` (`internal/adapter`) and is
added to `allAdapters` in `internal/custom`:

- `Supported()` — whether it can ever work on this device, and if not why (AutoHotkey: only on
  Windows). Something that can be installed does not count: AutoHotkey not installed is supported,
  and describing its scripts says how to install it. An unsupported adapter is left out everywhere
  (`custom.Adapters()`): not offered by `custom-tools add` or its file dialog, adding a file only it
  handles fails with why, and custom tools of it (e.g. from another device's data folder) are not in
  the tool list and fail with why; `custom-tools list` still shows them. Only the `custom-tools`
  menu's "View unsupported adapters" lists them (`custom.Unsupported()`). Its name stays reserved.
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

AutoHotkey v2 (Windows only): scripts declare no parameters, so args are passed on as given (the
script's `A_Args`). The summary is the `;@Ahk2Exe-SetDescription` directive, else the first paragraph
of the comment at the top (`;` lines or a `/* */` block, after `#` directives). Nothing of the script
runs to read it (not even `/Validate`, since `#DllLoad` would load a DLL). `#Requires AutoHotkey v1` is
refused; `#Requires … 32-bit` runs it with `AutoHotkey32.exe`. It runs as `AutoHotkey64.exe
/ErrorStdOut=UTF-8 <path> <args…>`, from the `v2` folder of the install (`InstallDir` under
`Software\AutoHotkey` in the registry, else `Program Files\AutoHotkey` or `%LOCALAPPDATA%\Programs\AutoHotkey`),
else from PATH (a portable copy). Not found: describing fails with how to install it
(`winget install AutoHotkey.AutoHotkey` or the download page), shown by `custom-tools add` / `list`, in
the window's tool list and on a run. Not interactive: it asks in its own windows (`InputBox`, `MsgBox`),
its output to `*` (`FileAppend`) shows in the run; a script that stays running (hotkeys, a Gui) keeps
its run going until it exits.

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
- `QUOTA_WARN_HOURS` (default `1`) — a month over and short of its quota by less than this many
  hours shows as a warning (yellow), not red; `0`: always red. Shown under Settings > Widgets (AEXT quota):
  a setting with `Setting.Widget` set is listed there, by widget, instead of on General.
- `TZ_OFFSET_HOURS` (default `auto`) — timezone all dates are computed in (ranges, "today", worklog days,
  file timestamps): `auto` follows the device's (daylight saving included), a number pins a UTC offset;
  quarter hours allowed, e.g. `5.5` = UTC+5:30. The settings page picks it from a list, each offset
  named by well-known places on it now (`tzZones` in `settings.js`, daylight saving included), e.g.
  "Berlin, Paris, Madrid, Rome (UTC+2)", and the device's timezone by its place; it warns when a pinned
  offset is not the device's. The setting stays an offset: a pinned one does not follow daylight saving.

The window's header shows the current values, the settings file path, the credential store in use and the data folder.

## Credentials

`internal/secrets` stores the AEXT session, the Google login, `JIRA_TOKEN` and `GOOGLE_CLIENT_SECRET` (settings
with `Credential` set in `internal/settings/env.go`) behind one `Store` interface, in the OS credential store:

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
