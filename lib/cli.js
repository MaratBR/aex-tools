import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { err as color } from './color.js';
import { takeGlobalArgs } from './paths.js';

export function printError(err) {
  console.error(process.env.DEBUG ? err : `${color.red('error:')} ${err.message}`);
}

// Wraps a script's main so errors print cleanly and set a non-zero exit code.
// Global args (--data-dir, read by lib/config.js) are removed before main sees them.
export function run(main) {
  Promise.resolve()
    .then(() => main(takeGlobalArgs(process.argv.slice(2)).rest))
    .catch((err) => {
      printError(err);
      process.exitCode = 1;
    });
}

// True when the module at `url` was started directly with node (never inside the bundled exe,
// where import.meta.url is undefined).
export const isEntry = (url) =>
  Boolean(url && process.argv[1]) && path.resolve(process.argv[1]) === fileURLToPath(url);
