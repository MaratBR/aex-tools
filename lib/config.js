import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// All dates (ranges, "today", worklog days, file timestamps) are computed in this fixed offset.
export const TZ_OFFSET_HOURS = 7;

// Jira project key -> project code used in the CSV. Unmapped keys are used as-is.
export const PROJECT_MAP = {
  CM: 'CMOS',
};

// AEXT working-days calendar country and daily quota.
export const WORKING_DAYS_COUNTRY = 'RU';
export const HOURS_PER_DAY = 8;

// Leave statuses: pending | approved | declined | cancelled.
// Ignored ones do not reduce the quota; anything else not "approved" counts, with a warning.
export const IGNORED_LEAVE_STATUSES = ['declined', 'cancelled'];

export const JIRA_EXPORT_DIR = path.join(repoRoot, 'output', 'jira-export');
export const REPO_ZIP_DIR = path.join(repoRoot, 'output', 'repo-zip');
export const AEXT_SESSION_FILE = process.env.AEXT_SESSION_FILE || path.join(repoRoot, 'session.txt');
