import { requireAuthSetting, requireEnv } from './env.js';
import { addDays, dayOf, dayStartMs } from './dates.js';
import { isArray, readJson } from './http.js';

// Minimal Jira Cloud REST v3 client (API token basic auth).
export async function createJiraClient() {
  const baseUrl = requireEnv('JIRA_BASE_URL').replace(/\/+$/, '');
  const email = await requireAuthSetting('JIRA_EMAIL');
  const token = await requireAuthSetting('JIRA_TOKEN');
  const auth = Buffer.from(`${email}:${token}`).toString('base64');

  async function request(method, path, { query, body, validate } = {}) {
    const url = new URL(baseUrl + path);
    for (const [k, v] of Object.entries(query ?? {})) {
      if (v !== undefined) url.searchParams.set(k, String(v));
    }
    const res = await fetch(url, {
      method,
      headers: {
        Authorization: `Basic ${auth}`,
        Accept: 'application/json',
        ...(body ? { 'Content-Type': 'application/json' } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
    });
    return readJson(`Jira ${method} ${url.pathname}`, res, { validate });
  }

  return {
    get: (path, query, validate) => request('GET', path, { query, validate }),
    post: (path, body, validate) => request('POST', path, { body, validate }),
  };
}

const isSearchPage = (d) => isArray(d?.issues);
const isWorklogPage = (d) =>
  isArray(d?.worklogs) &&
  typeof d.total === 'number' &&
  d.worklogs.every((w) => typeof w.started === 'string' && typeof w.timeSpentSeconds === 'number');

async function searchIssues(jira, jql, fields) {
  const issues = [];
  let nextPageToken;
  do {
    const page = await jira.post('/rest/api/3/search/jql', { jql, fields, maxResults: 100, nextPageToken }, isSearchPage);
    issues.push(...page.issues);
    nextPageToken = page.nextPageToken;
  } while (nextPageToken);
  return issues;
}

async function fetchIssueWorklogs(jira, issueKey, startedAfter, startedBefore) {
  const worklogs = [];
  let startAt = 0;
  for (;;) {
    const page = await jira.get(
      `/rest/api/3/issue/${issueKey}/worklog`,
      { startAt, maxResults: 5000, startedAfter, startedBefore },
      isWorklogPage,
    );
    worklogs.push(...page.worklogs);
    startAt += page.worklogs.length;
    if (page.worklogs.length === 0 || startAt >= page.total) return worklogs;
  }
}

async function mapLimit(items, limit, fn) {
  const results = new Array(items.length);
  let next = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (next < items.length) {
      const i = next++;
      results[i] = await fn(items[i]);
    }
  });
  await Promise.all(workers);
  return results;
}

// Current user's worklogs whose start falls on a day in [from, to] (days in the configured TZ).
export async function fetchMyWorklogs(jira, { from, to }) {
  const me = await jira.get('/rest/api/3/myself', undefined, (d) => typeof d?.accountId === 'string');
  const startedAfter = dayStartMs(from);
  const startedBefore = dayStartMs(addDays(to, 1));

  // worklogDate uses the Jira profile timezone, so widen by a day and filter exactly below.
  const jql = `worklogAuthor = currentUser() AND worklogDate >= "${addDays(from, -1)}" AND worklogDate <= "${addDays(to, 1)}"`;
  const issues = await searchIssues(jira, jql, ['project']);

  const perIssue = await mapLimit(issues, 5, async (issue) => {
    const logs = await fetchIssueWorklogs(jira, issue.key, startedAfter, startedBefore);
    return logs
      .filter((w) => w.author?.accountId === me.accountId)
      .map((w) => ({
        issueKey: issue.key,
        projectKey: issue.fields.project?.key ?? issue.key.split('-')[0],
        day: dayOf(w.started),
        seconds: w.timeSpentSeconds,
      }))
      .filter((w) => w.day >= from && w.day <= to);
  });
  return perIssue.flat();
}
