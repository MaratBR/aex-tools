import { spawn } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { isEntry, run } from '../lib/cli.js';
import { DATA_DIR } from '../lib/config.js';

export const summary = 'Open the data folder: settings, AEXT session, CSV exports';

const HELP = `Usage: data-folder [--print]

Opens the data folder in the file manager:
  ${DATA_DIR}
It holds .env.config, session.txt and output/. Change it with --data-dir <dir> or AEX_DATA_DIR.

  --print  Only print the path`;

const OPENER = { win32: 'explorer.exe', darwin: 'open' }[process.platform] ?? 'xdg-open';

export async function main(argv) {
  if (argv.includes('--help') || argv.includes('-h')) {
    console.log(HELP);
    return;
  }
  console.log(DATA_DIR);
  if (argv.includes('--print')) return;
  mkdirSync(DATA_DIR, { recursive: true });
  // Detached: the file manager outlives this process (explorer.exe also exits non-zero on success).
  spawn(OPENER, [DATA_DIR], { detached: true, stdio: 'ignore' }).on('error', () => {}).unref();
}

if (isEntry(import.meta.url)) run(main);
