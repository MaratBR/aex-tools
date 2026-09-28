import { settingValue } from './env.js';

export * from './paths.js';

// Hours per working day (quota) and the timezone all dates are computed in (ranges, "today",
// worklog days, file timestamps). App settings HOURS_PER_DAY / TZ_OFFSET_HOURS (see lib/env.js),
// read on every call so changes made by configure apply without a restart.
export const hoursPerDay = () => Number(settingValue('HOURS_PER_DAY'));
export const tzOffsetHours = () => Number(settingValue('TZ_OFFSET_HOURS'));

// Jira project key -> project code used in the CSV. Unmapped keys are used as-is.
export const PROJECT_MAP = {
  CM: 'CMOS',
};

// AEXT working-days calendar country.
export const WORKING_DAYS_COUNTRY = 'RU';

// Leave statuses: pending | approved | declined | cancelled.
// Ignored ones do not reduce the quota; anything else not "approved" counts, with a warning.
export const IGNORED_LEAVE_STATUSES = ['declined', 'cancelled'];
