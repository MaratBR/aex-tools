// AEXT time-tracking API client. Auth is email + one-time code; the auth_token cookie is
// cached in session.txt and refreshed automatically when the server rejects it.
import { existsSync, readFileSync } from 'node:fs';
import { writeFile, rm } from 'node:fs/promises';
import { AEXT_SESSION_FILE, WORKING_DAYS_COUNTRY } from './config.js';
import { requireEnv } from './env.js';
import { dumpResponse, isArray, readJson } from './http.js';
import { err as color } from './color.js';
import { ask, assertInteractive } from './prompt.js';

export function createAextClient() {
  const baseUrl = requireEnv('AEXT_BASE_URL').replace(/\/+$/, '');
  const email = requireEnv('AEXT_EMAIL');
  let cookie = existsSync(AEXT_SESSION_FILE) ? readFileSync(AEXT_SESSION_FILE, 'utf8').trim() : '';

  function send(method, path, { query, body } = {}) {
    const url = new URL(baseUrl + path);
    for (const [k, v] of Object.entries(query ?? {})) url.searchParams.set(k, String(v));
    return fetch(url, {
      method,
      headers: {
        Accept: 'application/json',
        ...(body ? { 'Content-Type': 'application/json' } : {}),
        ...(cookie ? { Cookie: cookie } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async function login() {
    assertInteractive('AEXT login');
    let res = await send('POST', '/api/auth/request-code', { body: { email } });
    await readJson('AEXT POST /api/auth/request-code', res, { expect: [200, 204] });

    const code = await ask(`AEXT login code sent to ${email}. Code: `);
    const label = 'AEXT POST /api/auth/verify-code';
    res = await send('POST', '/api/auth/verify-code', { body: { email, code, remember: true } });
    const data = await readJson(label, res, { validate: (d) => d?.ok === true });

    const authCookie = res.headers
      .getSetCookie()
      .map((c) => c.split(';')[0].trim())
      .find((c) => c.startsWith('auth_token='));
    if (!authCookie) {
      dumpResponse(label, res, JSON.stringify(data), 'no auth_token cookie');
      throw new Error(`${label}: no auth_token cookie (details above)`);
    }

    cookie = authCookie;
    await writeFile(AEXT_SESSION_FILE, `${cookie}\n`, 'utf8');
    console.error(color.green('AEXT login ok, session saved.'));
  }

  async function request(method, path, { validate, ...opts } = {}) {
    if (!cookie) await login();
    let res = await send(method, path, opts);
    if (res.status === 401 || res.status === 403) {
      await res.body?.cancel();
      console.error(color.yellow('AEXT session expired, logging in again.'));
      cookie = '';
      await rm(AEXT_SESSION_FILE, { force: true });
      await login();
      res = await send(method, path, opts);
    }
    return readJson(`AEXT ${method} ${path}`, res, { validate });
  }

  return {
    // -> [{ date: 'YYYY-MM-DD', hours_total: number }], only days with entries.
    getTimeSummary: (startDate, endDate) =>
      request('GET', '/api/time-entries/summary', {
        query: { start_date: startDate, end_date: endDate },
        validate: (d) => isArray(d) && d.every((s) => typeof s.date === 'string' && typeof s.hours_total === 'number'),
      }),

    // -> [{ date, is_working_day, day_type, description, ... }] for every day in range.
    getWorkingDays: async (startDate, endDate, countryCode = WORKING_DAYS_COUNTRY) =>
      (
        await request('GET', '/api/working-days/period', {
          query: { start_date: startDate, end_date: endDate, country_code: countryCode },
          validate: (d) => isArray(d?.days) && d.days.every((x) => typeof x.date === 'string' && typeof x.is_working_day === 'boolean'),
        })
      ).days,

    // -> [{ id, user: { email }, leave_type, start, end, status, created: { user: { email } }, ... }]
    // Personal fields (names, position) come back redacted; rely only on ids, emails, dates, status.
    getMyLeaves: (startDate, endDate) =>
      request('GET', '/api/leaves/my-requests', {
        query: { start_date: startDate, end_date: endDate },
        validate: (d) => isArray(d) && d.every((l) => l.id != null && typeof l.start === 'string' && typeof l.end === 'string'),
      }),

    // entries: [{ date, project, description, hours_total }] -> { imported_count, entries }
    importEntries: (entries) =>
      request('POST', '/api/time-entries/import', {
        body: { entries },
        validate: (d) => typeof d?.imported_count === 'number',
      }),
  };
}
