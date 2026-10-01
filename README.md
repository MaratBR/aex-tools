# aex-tools

`aex` is one app for everyday AEX work: a window with every tool and a Home page of widgets. Each tool
also runs in a terminal as `aex-cli <tool> [args]` (`aex` on macOS and Linux).

## Get started

1. Run the installer (`aex-<version>-windows-<arch>-setup.exe`, in the release zip): it installs aex for you
   only, no admin needed, into `%LOCALAPPDATA%\Programs\aex`, and adds it to the Start menu. Or build it:
   see [BUILD.md](BUILD.md). You get `dist\aex.exe` (the window), `dist\aex-cli.exe` (the
   terminal) and `dist\plugins\`. Keep them together. It needs the WebView2 runtime, which Windows 11 already has.
2. Double-click `aex.exe`. On first start it asks a few questions, such as whether to start with Windows
   and whether to add it to the Start menu.
3. Open **Settings** in the sidebar to set your AEXT and Jira logins, hours per day and time zone. If a
   tool needs a setting you haven't set, it asks for it when you run it.

In a terminal: `aex-cli <tool> --help` shows what a tool takes, and `aex-cli --help` lists the tools.
`aex-cli` without a tool opens the window too.

## Tools

- **worklog-sync** exports your Jira worklogs for a date range to a CSV and then sends them to AEXT.
  It skips entries AEXT already has and fixes ones whose hours changed. It suggests a range from the
  days missing in AEXT. You can also give one: `--range "this week"`, `last month`, `09.20-09.30`.
- **quota** shows this month's and last month's AEXT hours against your quota: how far behind or ahead
  you are, and how many hours a day you need to finish. Leaves are taken into account.
- **account** shows who you are logged in as in AEXT, Jira and Google, and lets you log in or out.
- **configure** changes settings (the Settings page in the window). It can also start aex when you log
  in, add aex to the Start menu, and wipe settings or all data.
- **plugins** manages the tools in the `plugins` folder.
- **custom-tools** adds your own scripts (PowerShell `.ps1`, AutoHotkey v2 `.ahk`) as tools, grouped by
  kind (`aex powershell <name>`).
  aex asks for their parameters. Adding one asks which adapter runs it (or picks it by the file) and
  offers to approve it right away. Adapters that cannot work on your OS (AutoHotkey off Windows) are
  hidden; its menu's "View unsupported adapters" says why.
- **cm-release** (a plugin) has the CM release steps: `pull-all`, `prepare-release`, `merge-prod` and
  `jira-handoff`.

## Home

Home is a grid of widgets. Use **Add widget** to add one, and **Edit** to move or resize them: drag a
widget to any cell, leaving gaps if you like; widgets in the way move down. A widget
this aex does not have (e.g. from another version) stays on the page with a warning, to remove in Edit.

- **AEXT quota**: this month's hours at a glance, with buttons to run worklog-sync, including one that
  syncs this week (Monday through today). ‹ › show other months. Leave (approved green, pending blue)
  and holidays show on the bar and are skipped, with a warning when you logged hours on them. A month
  short by less than an hour shows in yellow (Settings > Widgets to change it).
- **Google Calendar**: what's on now and coming up in one calendar.
- **Jira tickets**: your open tickets, tickets where you are in a field you pick, or a JQL query; add
  tabs for several lists. To Do tickets not updated for 2 weeks are faded.
- **Git status**: whether the git repos you pick are clean (the CM repos by default, with the cm-release
  plugin), with a button to open your git client (Fork, GitHub Desktop, GitKraken, Sourcetree, Sublime
  Merge, SmartGit or TortoiseGit).
- **Clock**: the time in one or two time zones (Central Time by default), with the day when it isn't
  today here; 24-hour or AM/PM.
- **Shortcuts**: buttons that run the tools you pick, each with a name, an icon (or an emoji) or both;
  once the tool succeeds you are back on Home (a button can stay on Runs instead).
  Several show as a grid, with a title if you give one. They are edited in a popup, so a small widget
  has room for it.
- **Cat** and **2048**, for a break.

Tools and widgets can show reminders: a message on top of every window on every screen, with a soft
chime, until you close it (× at the top right) on any of them. It never takes the focus from what you
are typing in. Links in it open in your default browser, or in the one a link names:
`aex+brave://google.com` opens in Brave (also chrome, edge, firefox, vivaldi, yandex, opera, …), or
the default browser when that one is not installed. A link stays clickable after you use it, unless it
has a `!` before it, which closes the reminder once the link opens: `[Join the call](!https://…)`.

Set up your own in **Settings > Reminders**: a time, the days of the week, a title and a message (with
links). They show while aex runs, so let it start when you log in (Settings > General). An **extra
urgent** one is marked red and chimes twice, then twice again every 30 seconds until you close it or
10 minutes pass.

## Where your data is kept

- Settings and output files are in the data folder, `%APPDATA%\aex` by default. You can open it from the
  sidebar, or use another one with `--data-dir <dir>`.
- Logins (the AEXT session, Jira token and Google login) are kept in the Windows Credential Manager
  (the Keychain on macOS), not in plain files.

## Safety

aex runs a plugin or a custom script only after you approve that exact file. If the file changes, aex
asks you again. The plugins built together with aex from this repo are approved already. A plugin gets
only the logins you allow it to use.

## About and license

aex is licensed under the [Apache License 2.0](LICENSE); see also [NOTICE](NOTICE). **Settings > About**
shows the version, when and from which commit it was built, the plugins pre-approved in it, and the
licenses of the open source it includes. It and the bottom of the sidebar also say how many commits
the build is behind master on GitHub (checked every few hours, or with Check now).

## Working on aex

Build steps are in [BUILD.md](BUILD.md). How the code works is in [AGENTS.md](AGENTS.md), which is
written for coding agents but is readable by people too.
