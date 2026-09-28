// Calendar-day helpers. A "day" is a YYYY-MM-DD string in the TZ_OFFSET_HOURS zone.
import { TZ_OFFSET_HOURS } from './config.js';

const OFFSET_MS = TZ_OFFSET_HOURS * 3600_000;

export const TZ_LABEL = `UTC${TZ_OFFSET_HOURS >= 0 ? '+' : '-'}${Math.abs(TZ_OFFSET_HOURS)}`;

const shifted = (ms) => new Date(ms + OFFSET_MS).toISOString();

export const today = () => shifted(Date.now()).slice(0, 10);

// e.g. 2026-09-28T134142
export const fileTimestamp = () => shifted(Date.now()).slice(0, 19).replace(/:/g, '');

// Day of an absolute timestamp such as Jira's "2026-09-28T09:00:00.000+0200".
export const dayOf = (timestamp) => shifted(Date.parse(timestamp)).slice(0, 10);

// Epoch ms of 00:00 on the given day.
export const dayStartMs = (day) => Date.parse(`${day}T00:00:00Z`) - OFFSET_MS;

export function addDays(day, n) {
  const d = new Date(`${day}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}

export function eachDay(from, to) {
  const days = [];
  for (let d = from; d <= to; d = addDays(d, 1)) days.push(d);
  return days;
}

export const monthStart = (day) => `${day.slice(0, 8)}01`;
export const prevMonthStart = (day) => monthStart(addDays(monthStart(day), -1));
export const monthEnd = (day) => addDays(monthStart(addDays(monthStart(day), 31)), -1);

function weekStart(day) {
  const dow = new Date(`${day}T00:00:00Z`).getUTCDay();
  return addDays(day, -((dow + 6) % 7));
}

function makeDay(y, m, d) {
  const day = `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
  const check = new Date(`${day}T00:00:00Z`);
  if (Number.isNaN(check.getTime()) || check.toISOString().slice(0, 10) !== day) return null;
  return day;
}

// 2026.09.20 | 26.09.20 | 09.20 (year defaults to defaultYear)
function parseDay(text, defaultYear) {
  let m = text.match(/^(\d{4})\.(\d{1,2})\.(\d{1,2})$/);
  if (m) return makeDay(+m[1], +m[2], +m[3]);
  m = text.match(/^(\d{2})\.(\d{1,2})\.(\d{1,2})$/);
  if (m) return makeDay(2000 + +m[1], +m[2], +m[3]);
  m = text.match(/^(\d{1,2})\.(\d{1,2})$/);
  if (m) return makeDay(defaultYear, +m[1], +m[2]);
  return null;
}

/**
 * Parses a user range expression relative to `now` (a day string).
 * Accepts: today, yesterday, this week, last week, this month, last month,
 * a single day (2026.09.20, 26.09.20, 09.20) or two days joined by "-" (09.20-09.30).
 * Returns { from, to } or throws.
 */
export function parseRange(input, now = today()) {
  const text = input.trim().toLowerCase().replace(/\s+/g, ' ');
  switch (text) {
    case 'today':
      return { from: now, to: now };
    case 'yesterday': {
      const d = addDays(now, -1);
      return { from: d, to: d };
    }
    case 'this week':
      return { from: weekStart(now), to: now };
    case 'last week': {
      const from = addDays(weekStart(now), -7);
      return { from, to: addDays(from, 6) };
    }
    case 'this month':
      return { from: monthStart(now), to: now };
    case 'last month':
      return { from: prevMonthStart(now), to: addDays(monthStart(now), -1) };
  }

  const year = +now.slice(0, 4);
  const parts = text.split(/\s*-\s*/);
  if (parts.length > 2) throw new Error(`Cannot parse range "${input}"`);
  const from = parseDay(parts[0], year);
  if (!from) throw new Error(`Cannot parse range "${input}"`);
  let to = from;
  if (parts.length === 2) {
    const fromYear = +from.slice(0, 4);
    to = parseDay(parts[1], fromYear);
    // Year-less end before start wraps into next year: 12.30-01.02.
    if (to && to < from && /^\d{1,2}\.\d{1,2}$/.test(parts[1])) to = parseDay(parts[1], fromYear + 1);
  }
  if (!to) throw new Error(`Cannot parse range "${input}"`);
  if (from > to) throw new Error(`Range start ${from} is after end ${to}`);
  return { from, to };
}
