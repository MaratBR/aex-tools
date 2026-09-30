# aex-tools

`aex` is one app for everyday AEX work: a window with every tool and a Home page of widgets. Each tool
also runs in a terminal as `aex <tool> [args]`.

## Get started

1. Build it: see [BUILD.md](BUILD.md). You get `dist\aex.exe` and `dist\plugins\`. Keep the `plugins`
   folder next to the exe. It needs the WebView2 runtime, which Windows 11 already has.
2. Double-click `aex.exe`. On first start it asks a few questions, such as whether to start with Windows
   and whether to add it to the Start menu.
3. Open **Settings** in the sidebar to set your AEXT and Jira logins, hours per day and time zone. If a
   tool needs a setting you haven't set, it asks for it when you run it.

In a terminal: `aex <tool> --help` shows what a tool takes, and `aex --help` lists the tools.

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
- **custom-tools** adds your own scripts (PowerShell `.ps1`) as tools. aex asks for their parameters.
- **cm-release** (a plugin) has the CM release steps: `pull-all`, `prepare-release`, `merge-prod` and
  `jira-handoff`.

## Home

Home is a grid of widgets. Use **Add widget** to add one, and **Edit** to move or resize them.

- **AEXT quota**: this month's hours at a glance, with buttons to run worklog-sync, including one that
  syncs this week (Monday through today).
- **Google Calendar**: what's on now and coming up in one calendar.
- **Jira tickets**: your open tickets, tickets where you are in a field you pick, or a JQL query; add
  tabs for several lists. To Do tickets not updated for 2 weeks are faded.
- **CM repos state**: whether every CM repo is clean, with a button to open your git client (Fork,
  GitHub Desktop, GitKraken, Sourcetree, Sublime Merge, SmartGit or TortoiseGit).
- **Clock**: the time in one or two time zones (Central Time by default), with the day when it isn't
  today here; 24-hour or AM/PM.
- **Cat** and **2048**, for a break.

## Where your data is kept

- Settings and output files are in the data folder, `%APPDATA%\aex` by default. You can open it from the
  sidebar, or use another one with `--data-dir <dir>`.
- Logins (the AEXT session, Jira token and Google login) are kept in the Windows Credential Manager
  (the Keychain on macOS), not in plain files.

## Safety

aex runs a plugin or a custom script only after you approve that exact file. If the file changes, aex
asks you again. The plugins built together with aex from this repo are approved already. A plugin gets
only the logins you allow it to use.

## Working on aex

Build steps are in [BUILD.md](BUILD.md). How the code works is in [AGENTS.md](AGENTS.md), which is
written for coding agents but is readable by people too.
