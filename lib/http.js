// Response handling shared by API clients: anything unexpected (status, non-JSON body, wrong shape)
// is dumped to stderr with status, URL, relevant headers and a capped body, then thrown.

import { err as color } from './color.js';

const MAX_DUMP_BYTES = 300 * 1024;

const RELEVANT_HEADERS = [
  /^content-(type|length|encoding)$/,
  /^(date|server|location|retry-after|www-authenticate|allow|via)$/,
  /^set-cookie$/,
  /request-?id|trace-?id|correlation-?id|^cf-ray$|^x-arequestid$/,
  /ratelimit/,
  /^x-(seraph-loginreason|ausername|cache|error.*)$/,
];

// Keep cookie names and attributes, hide values.
const redactCookie = (c) => c.replace(/^([^=;]+)=[^;]*/, '$1=<redacted>');

export function dumpResponse(label, res, body, reason) {
  const lines = [
    color.red(`--- ${label}: ${reason} ---`),
    color.bold(`HTTP ${res.status} ${res.statusText}`),
    `${color.dim('URL:')} ${res.url}`,
  ];
  for (const [name, value] of res.headers) {
    if (name === 'set-cookie') continue;
    if (RELEVANT_HEADERS.some((re) => re.test(name))) lines.push(`${color.dim(`${name}:`)} ${value}`);
  }
  for (const c of res.headers.getSetCookie()) lines.push(`${color.dim('set-cookie:')} ${redactCookie(c)}`);

  const bytes = Buffer.from(body ?? '', 'utf8');
  lines.push('', bytes.length ? bytes.subarray(0, MAX_DUMP_BYTES).toString('utf8') : color.dim('<empty body>'));
  if (bytes.length > MAX_DUMP_BYTES) {
    lines.push(color.yellow(`[truncated: showing ${MAX_DUMP_BYTES} of ${bytes.length} bytes]`));
  }
  lines.push(color.red(`--- end ${label} ---`));
  console.error(lines.join('\n'));
}

/**
 * Reads a response and returns parsed JSON (null for an empty body).
 * Dumps and throws unless status is in `expect` (default: any 2xx) and `validate(data)` passes.
 */
export async function readJson(label, res, { expect, validate } = {}) {
  const body = await res.text().catch((err) => `<failed to read body: ${err.message}>`);
  const fail = (reason) => {
    dumpResponse(label, res, body, reason);
    return new Error(`${label}: ${reason} (HTTP ${res.status}, details above)`);
  };

  if (expect ? !expect.includes(res.status) : !res.ok) throw fail('unexpected status');
  let data = null;
  if (body.trim()) {
    try {
      data = JSON.parse(body);
    } catch {
      throw fail('response is not JSON');
    }
  }
  if (validate && !validate(data)) throw fail('unexpected response shape');
  return data;
}

export const isArray = (d) => Array.isArray(d);
