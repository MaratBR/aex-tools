// Monthly hours quota from AEXT working days, leaves and logged time.
import { IGNORED_LEAVE_STATUSES, WORKING_DAYS_COUNTRY, hoursPerDay } from './config.js';
import { eachDay, monthEnd, monthStart, prevMonthStart } from './dates.js';
import { out } from './color.js';

const isIgnored = (leave) => IGNORED_LEAVE_STATUSES.includes(leave.status);

const sameEmail =(a, b) => (a ?? '').trim().toLowerCase() === (b ?? '').trim().toLowerCase();

// Leave requests overlapping [from, to]. Fetched per calendar year; ids deduped.
async function fetchLeaves(aext, from, to) {
  const years = [...new Set([from.slice(0, 4), to.slice(0, 4)])];
  const lists = await Promise.all(years.map((y) => aext.getMyLeaves(`${y}-01-01`, `${y}-12-31`)));
  const byId = new Map(lists.flat().map((l) => [l.id, l]));
  return [...byId.values()].filter((l) => l.start <= to && l.end >= from);
}

function leaveWarnings(leaves) {
  const warnings = [];
  for (const l of leaves) {
    const what = `Leave #${l.id} (${l.leave_type}, ${l.start}..${l.end})`;
    if (isIgnored(l)) warnings.push(`${what} is "${l.status}", ignored.`);
    else if (l.status !== 'approved') warnings.push(`${what} is "${l.status}", counted as leave anyway.`);
    const creator = l.created?.user?.email;
    if (creator && !sameEmail(creator, l.user?.email)) {
      warnings.push(`${what} was created by ${creator}, not ${l.user?.email ?? 'you'}.`);
    }
  }
  return warnings;
}

/**
 * Loads last month + current month from AEXT.
 * Returns {
 *   workingDays: Set<day>  working days per calendar, minus leave days,
 *   leaveDays: Set<day>    calendar working days covered by a leave,
 *   hoursByDay: Map<day, hours>,
 *   warnings: string[]
 * }.
 */
export async function fetchMonths(aext, now) {
  const from = prevMonthStart(now);
  const to = monthEnd(now);
  const [days, summary, leaves] = await Promise.all([
    aext.getWorkingDays(from, to),
    aext.getTimeSummary(from, to),
    fetchLeaves(aext, from, to),
  ]);

  const hoursByDay = new Map();
  for (const s of summary) {
    if (s.hours_total > 0) hoursByDay.set(s.date, (hoursByDay.get(s.date) ?? 0) + s.hours_total);
  }

  const calendarWorking = new Set(days.filter((d) => d.is_working_day).map((d) => d.date));
  const leaveDays = new Set(
    leaves
      .filter((l) => !isIgnored(l))
      .flatMap((l) => eachDay(l.start > from ? l.start : from, l.end < to ? l.end : to))
      .filter((d) => calendarWorking.has(d)),
  );
  const workingDays = new Set([...calendarWorking].filter((d) => !leaveDays.has(d)));

  return { workingDays, leaveDays, hoursByDay, warnings: leaveWarnings(leaves) };
}

export function computeMonthQuota(month, now, { workingDays, leaveDays, hoursByDay }) {
  const from = monthStart(month);
  const to = monthEnd(month);
  const inMonth = (d) => d >= from && d <= to;
  const days = [...workingDays].filter(inMonth);
  const logged = [...hoursByDay].filter(([d]) => inMonth(d)).reduce((sum, [, h]) => sum + h, 0);
  const expected = days.length * hoursPerDay();

  // Today counts as remaining, not as already due.
  const daysDue = days.filter((d) => d < now).length;
  const daysLeft = days.filter((d) => d >= now).length;
  const remaining = Math.max(0, expected - logged);

  return {
    month: from.slice(0, 7),
    workingDays: days.length,
    leaveDays: [...leaveDays].filter(inMonth).length,
    expected,
    logged,
    percent: expected ? (logged / expected) * 100 : 100,
    expectedToDate: daysDue * hoursPerDay(),
    behind: daysDue * hoursPerDay() - logged,
    remaining,
    daysLeft,
    perDayLeft: daysLeft ? remaining / daysLeft : null,
  };
}

const h = (n) => `${n.toFixed(2)}h`;

// Styled for stdout.
export function formatQuota(now, data) {
  const lines = [out.bold(`Quota`) + out.dim(` (${hoursPerDay()}h/working day, ${WORKING_DAYS_COUNTRY} calendar)`)];
  for (const month of [prevMonthStart(now), now]) {
    const q = computeMonthQuota(month, now, data);
    const finished = q.daysLeft === 0;
    const leave = q.leaveDays ? out.dim(` (+${q.leaveDays} on leave)`) : '';
    // Only a finished month's % is a verdict; mid-month it is just progress.
    const pct = `${q.percent.toFixed(1)}%`;
    const pctStyled = finished ? (q.remaining > 0 ? out.red(pct) : out.green(pct)) : out.bold(pct);
    lines.push(
      `${out.bold(q.month)}: ${q.workingDays} working days${leave}, ${h(q.expected)} expected, ${h(q.logged)} logged, ${pctStyled}`,
    );
    if (finished) {
      if (q.remaining > 0) lines.push(`  ${out.red(`${h(q.remaining)} short`)}`);
      continue;
    }
    const status = q.behind > 0 ? out.red(`${h(q.behind)} behind`) : out.green(`${h(-q.behind)} ahead`);
    lines.push(`  By today: ${h(q.expectedToDate)} due, ${status}`);
    const perDay = `${h(q.perDayLeft)}/day`;
    lines.push(
      `  Remaining: ${h(q.remaining)} over ${q.daysLeft} working day(s) incl. today -> ` +
        (q.perDayLeft > hoursPerDay() ? out.yellow(perDay) : out.green(perDay)),
    );
  }
  for (const w of data.warnings) lines.push(`${out.yellow('Warning:')} ${w}`);
  return lines.join('\n');
}
