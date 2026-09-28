# aex-tools

Personal work tooling. Node scripts (no build step, ESM, Node >= 20.12), each runnable from PowerShell.

## Layout

- `scripts/<name>.js` — one script per tool
- `lib/` — shared helpers
  - `config.js` — timezone (`TZ_OFFSET_HOURS`, default UTC+7), Jira→CSV project map, output paths
  - `env.js` — loads `.env.private` + `.env`, prompts for missing secrets
  - `dates.js` — day math and range expressions
  - `jira.js`, `aext.js` — API clients
  - `color.js` — ANSI colors, only on a terminal; `NO_COLOR=1` disables, `FORCE_COLOR=1` forces
  - `http.js` — on any unexpected response (status, non-JSON, wrong shape) dumps status, URL,
    relevant headers (cookie values redacted) and body (capped at 300 KB) to stderr
  - `prompt.js`, `cli.js`, `csv.js`
- `bin/<name>.ps1` — PowerShell entrypoint per script (thin wrapper over `bin/_invoke.ps1`)
- `output/` — generated files (gitignored)

## worklog-sync

Exports Jira worklogs to `output/jira-export/<timestamp>.csv` (`date,project,hours,description`,
one row per day + issue), then offers to send it to AEXT.

Range: by default suggested from AEXT (first working day this month with no hours per the AEXT working-days calendar, through today;
offers an earlier start if last month has gaps). Decline, or use `--manual`, to type one:
`today`, `yesterday`, `this week`, `last week`, `this month`, `last month`, `2026.09.20`,
`26.09.20`, `09.20`, `09.20-09.30`. `--range <expr>` skips the prompts.

AEXT login is by emailed code. The session cookie is cached in `session.txt` (gitignored).

The CSV is re-read before import, so it can be edited before confirming. After import the quota is shown.

## quota

`.\bin\quota.ps1` shows last and current month: working days (AEXT calendar, `WORKING_DAYS_COUNTRY`),
expected hours (`HOURS_PER_DAY`), logged, % filled, hours behind as of today, and hours/day needed
over the remaining working days (today included).

Leaves (`/api/leaves/my-requests`, fetched per calendar year) remove their working days from the quota
and from gap detection in `worklog-sync`. `declined` and `cancelled` leaves are ignored with a warning
(`IGNORED_LEAVE_STATUSES` in `lib/config.js`); `pending` (or any other non-`approved` status) is counted, with a warning;
a leave created by someone other than you (by email) is also reported. Leave records are mostly
redacted, so only ids, emails, dates, type and status are used.

## zip-repo

`.\bin\zip-repo.ps1` zips the files committed at HEAD (`git archive`) into
`output/repo-zip/<repo>-<commit>-<timestamp>.zip`. Uncommitted, staged, untracked and gitignored files are left out.

## Usage

```powershell
.\bin\worklog-sync.ps1 --help
# or
npm run worklog-sync -- --help
```

Put `bin\` on `PATH` to call `worklog-sync.ps1` from anywhere.

## Adding a script

1. Create `scripts/foo.js` using `run(async (argv) => { ... })` from `lib/cli.js`.
2. Copy `bin/worklog-sync.ps1` to `bin/foo.ps1`, change the name.
3. Add `"foo": "node scripts/foo.js"` to `package.json` scripts.

## Config

- `.env`: shared non-secret config, committed.
- `.env.private`: secrets, gitignored, overrides `.env` (template: `.env.private.example`). Missing secrets are prompted for and can be saved there.
- Real environment variables override both.
- `.claude/settings.json` denies Claude Code access to `.env.private` and `session.txt`.
