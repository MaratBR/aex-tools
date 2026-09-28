import { run } from '../lib/cli.js';
import { createAextClient } from '../lib/aext.js';
import { today } from '../lib/dates.js';
import { fetchMonths, formatQuota } from '../lib/quota.js';

const HELP = `Usage: quota

Shows AEXT hours quota for last month and this month: working days, expected vs logged
hours, % filled, hours behind, and hours/day needed for the rest of this month.
Quota settings: HOURS_PER_DAY, WORKING_DAYS_COUNTRY in lib/config.js.`;

run(async (argv) => {
  if (argv.includes('--help') || argv.includes('-h')) {
    console.log(HELP);
    return;
  }
  const now = today();
  console.log(formatQuota(now, await fetchMonths(createAextClient(), now)));
});
