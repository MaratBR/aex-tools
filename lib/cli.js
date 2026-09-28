import { err as color } from './color.js';

// Wraps a script's main so errors print cleanly and set a non-zero exit code.
export function run(main) {
  main(process.argv.slice(2)).catch((err) => {
    console.error(process.env.DEBUG ? err : `${color.red('error:')} ${err.message}`);
    process.exitCode = 1;
  });
}
