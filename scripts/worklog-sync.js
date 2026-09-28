import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { parseArgs } from 'node:util';
import { isEntry, run } from '../lib/cli.js';
import { JIRA_EXPORT_DIR, PROJECT_MAP } from '../lib/config.js';
import { addDays, eachDay, fileTimestamp, monthStart, parseRange, prevMonthStart, today, tzLabel } from '../lib/dates.js';
import { createJiraClient, fetchMyWorklogs } from '../lib/jira.js';
import { createAextClient } from '../lib/aext.js';
import { fetchMonths, formatQuota } from '../lib/quota.js';
import { ask, assertInteractive, confirm } from '../lib/prompt.js';
import { parseCsv, toCsv } from '../lib/csv.js';
import { err, out } from '../lib/color.js';

export const summary = 'Export Jira worklogs to CSV, then send them to AEXT';

const HELP = `Usage: worklog-sync [--range <expr>] [--manual]

Exports your Jira worklogs for a date range to <data folder>/output/jira-export/<timestamp>.csv,
then offers to send the CSV to AEXT.

By default the range is suggested from AEXT: first working day of this month with no hours
logged, through today. Decline (or pass --manual) to type a range instead.

  --range   Range expression, skips all range prompts. Examples:
            today, yesterday, this week, last week, this month, last month,
            2026.09.20, 26.09.20, 09.20, 09.20-09.30
  --manual  Skip AEXT, ask for the range

Dates are in ${tzLabel()} (TZ_OFFSET_HOURS app setting, see configure).`;

const COLUMNS = [
  { header: 'date', value: (r) => r.day },
  { header: 'project', value: (r) => r.project },
  { header: 'hours', value: (r) => (r.seconds / 3600).toFixed(2) },
  { header: 'description', value: (r) => r.issueKey },
];

const fmtRange = ({ from, to }) => (from === to ? from : `${from} .. ${to}`);

async function askRange(now) {
  assertInteractive('Asking for a range');
  for (;;) {
    const input = await ask('Range (e.g. today, this week, 09.20, 09.20-09.30): ');
    try {
      return parseRange(input, now);
    } catch (e) {
      console.error(err.red(e.message));
    }
  }
}

const label = out.dim('AEXT:');

const emptyWorkingDays = (from, to, { workingDays, hoursByDay }) =>
  eachDay(from, to).filter((d) => workingDays.has(d) && !hoursByDay.has(d));

// Suggests { from, to } from AEXT, or null if nothing to import.
async function rangeFromAext(aext, now) {
  const curStart = monthStart(now);
  const prevStart = prevMonthStart(now);
  const prevEnd = addDays(curStart, -1);

  const data = await fetchMonths(aext, now);
  const logged = data.hoursByDay;
  for (const w of data.warnings) console.warn(`${err.yellow('Warning:')} ${w}`);
  if (data.leaveDays.size) console.log(`${label} leave days (not counted as gaps): ${[...data.leaveDays].sort().join(', ')}`);

  let from = emptyWorkingDays(curStart, now, data)[0] ?? null;
  if (from) console.log(`${label} first working day without hours this month: ${out.bold(from)}`);
  else console.log(`${label} every working day this month has hours logged.`);

  const prevEmpty = emptyWorkingDays(prevStart, prevEnd, data);
  if (prevEmpty.length) {
    const lastLogged = [...logged.keys()].filter((d) => d >= prevStart && d <= prevEnd).sort().at(-1);
    const alt = lastLogged ? addDays(lastLogged, 1) : prevStart;
    console.log(`${label} last month has ${out.yellow(prevEmpty.length)} working day(s) without hours: ${prevEmpty.join(', ')}`);
    if (alt < (from ?? addDays(now, 1))) {
      const prompt = lastLogged
        ? `Last logged day last month is ${lastLogged}. Start from ${alt} instead?`
        : `Nothing logged last month. Start from ${alt} instead?`;
      if (await confirm(prompt, false)) from = alt;
    }
  }

  if (!from) return null;
  const range = { from, to: now };
  const alreadyLogged = eachDay(from, now).filter((d) => logged.has(d));
  if (alreadyLogged.length) console.log(`${out.yellow('Note:')} these days in range already have AEXT hours: ${alreadyLogged.join(', ')}`);
  return range;
}

async function resolveRange(values, now, aext) {
  if (values.range) return parseRange(values.range, now);
  if (!values.manual) {
    assertInteractive('Choosing a range');
    if (await confirm('Determine range from AEXT?', true)) {
      const range = await rangeFromAext(aext(), now);
      if (range) return range;
      console.log('Nothing to import per AEXT, enter a range manually.');
    }
  }
  return askRange(now);
}

// One row per (day, issue), hours summed.
function toRows(worklogs) {
  const byKey = new Map();
  for (const w of worklogs) {
    const key = `${w.day}|${w.issueKey}`;
    const row = byKey.get(key) ?? {
      day: w.day,
      issueKey: w.issueKey,
      project: PROJECT_MAP[w.projectKey] ?? w.projectKey,
      seconds: 0,
    };
    row.seconds += w.seconds;
    byKey.set(key, row);
  }
  return [...byKey.values()].sort(
    (a, b) => a.day.localeCompare(b.day) || a.issueKey.localeCompare(b.issueKey, undefined, { numeric: true }),
  );
}

export async function main(argv) {
  const { values } = parseArgs({
    args: argv,
    options: {
      range: { type: 'string' },
      manual: { type: 'boolean', default: false },
      help: { type: 'boolean', short: 'h', default: false },
    },
  });
  if (values.help) {
    console.log(HELP);
    return;
  }

  const now = today();
  let aextClient;
  const aext = () => (aextClient ??= createAextClient());
  const range = await resolveRange(values, now, aext);
  console.log(`Range: ${out.bold(fmtRange(range))} ${out.dim(`(${tzLabel()})`)}`);
  if (!values.range && !(await confirm('Proceed?', true))) return;

  const jira = await createJiraClient();
  const rows = toRows(await fetchMyWorklogs(jira, range));
  if (!rows.length) {
    console.log('No Jira worklogs in range, nothing written.');
    return;
  }

  await mkdir(JIRA_EXPORT_DIR, { recursive: true });
  const csvFile = path.join(JIRA_EXPORT_DIR, `${fileTimestamp()}.csv`);
  await writeFile(csvFile, toCsv(COLUMNS, rows), 'utf8');
  const hours = rows.reduce((sum, r) => sum + r.seconds, 0) / 3600;
  console.log(`${rows.length} rows, ${out.bold(`${hours.toFixed(2)}h`)} -> ${out.cyan(csvFile)}`);

  if (process.stdin.isTTY && (await confirm(`Send CSV to AEXT? (edit ${path.basename(csvFile)} first if needed)`, false))) {
    await importCsv(aext(), csvFile);
    console.log(formatQuota(now, await fetchMonths(aext(), now)));
  }
}

if (isEntry(import.meta.url)) run(main);

// Re-reads the CSV so manual edits made before confirming are what gets sent.
async function importCsv(aext, file) {
  const entries = parseCsv(await readFile(file, 'utf8')).map((r, i) => {
    const hours = Number(r.hours);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(r.date) || !r.project || !Number.isFinite(hours) || hours <= 0) {
      throw new Error(`${path.basename(file)} row ${i + 2} is invalid: ${JSON.stringify(r)}`);
    }
    return { date: r.date, project: r.project, description: r.description, hours_total: hours };
  });
  if (!entries.length) throw new Error(`${path.basename(file)} has no rows`);

  const res = await aext.importEntries(entries);
  const ok = res.imported_count === entries.length;
  console.log(`${label} ${(ok ? out.green : out.yellow)(`imported ${res.imported_count} of ${entries.length} entries.`)}`);
  if (!ok) console.warn(`${err.yellow('Warning:')} AEXT imported a different number of entries than sent.`);
}
