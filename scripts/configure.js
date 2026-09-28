import { existsSync } from 'node:fs';
import { rm } from 'node:fs/promises';
import { isEntry, run } from '../lib/cli.js';
import { AEXT_SESSION_FILE } from '../lib/config.js';
import { SETTINGS, configEnvFile, envLayers, saveAppSettings, settingProblem } from '../lib/env.js';
import { ask, assertInteractive, confirm, promptSecret } from '../lib/prompt.js';
import { err, out } from '../lib/color.js';

export const summary = 'Set AEXT / Jira login, hours per day and timezone (app settings)';

const HELP = `Usage: configure

Asks for these settings and saves them to app settings (${configEnvFile}):
${SETTINGS.map((s) => `  ${s.name.padEnd(16)}${s.hint}${s.default ? ` (default ${s.default})` : ''}`).join('\n')}

App settings have the highest priority: they override .env, .env.private and real environment
variables, which remain fallbacks. Leave an answer empty to keep the current value, or enter "-" to
remove it from app settings (settings with a default are reset to it).`;

const mask = (value) => (value.length > 8 ? `${value.slice(0, 4)}…${value.slice(-4)}` : '****');

export async function main(argv) {
  if (argv.includes('--help') || argv.includes('-h')) {
    console.log(HELP);
    return;
  }
  assertInteractive('configure');
  console.log(`App settings: ${out.cyan(configEnvFile)}\n`);

  const updates = {};
  const changed = [];

  for (const { name, hint, secret, default: fallback } of SETTINGS) {
    const layers = envLayers();
    const inAppSettings = layers.at(-1).vars[name];
    const current = process.env[name];
    console.log(`${out.bold(name)} ${out.dim(`- ${hint}`)}`);
    if (current) console.log(out.dim(`  current: ${secret ? mask(current) : current}`));

    const removeLabel = fallback ? `- = reset to ${fallback}` : '- = remove';
    let input;
    for (;;) {
      input = (await (secret ? promptSecret : ask)(`  new value (empty = keep, ${removeLabel}): `)).trim();
      const problem = input && input !== '-' && settingProblem(name, input);
      if (!problem) break;
      console.error(`  ${err.red(problem)}`);
    }
    if (input === '-' && fallback) input = fallback;
    if (!input || (input === '-' && inAppSettings === undefined)) {
      console.log();
      continue;
    }

    if (input === '-') {
      updates[name] = null;
      delete process.env[name];
      // Fall back to the next source down, as a restart would.
      const next = layers.slice(0, -1).findLast((l) => l.vars[name])?.vars[name];
      if (next) process.env[name] = next;
    } else {
      const overridden = layers.slice(0, -1).filter((l) => l.vars[name]).map((l) => l.label);
      if (overridden.length) {
        console.warn(`  ${err.yellow('Warning:')} ${name} is also set in ${overridden.join(', ')}; app settings will override it.`);
        if (!(await confirm('  Save anyway?', true))) {
          console.log();
          continue;
        }
      }
      updates[name] = input;
      process.env[name] = input;
    }
    changed.push({ name, before: current, after: process.env[name] });
    console.log();
  }

  if (!changed.length) {
    console.log('Nothing changed.');
    return;
  }
  await saveAppSettings(updates);
  console.log(out.green(`Saved ${changed.map((c) => c.name).join(', ')} to ${configEnvFile}`));

  // The cached AEXT session belongs to the old email.
  const email = changed.find((c) => c.name === 'AEXT_EMAIL');
  if (email && email.before && email.before !== email.after && existsSync(AEXT_SESSION_FILE)) {
    if (await confirm(`AEXT email changed. Log out of AEXT (delete ${AEXT_SESSION_FILE})?`, true)) {
      await rm(AEXT_SESSION_FILE, { force: true });
      console.log('Logged out; the next AEXT tool will ask for a login code.');
    }
  }
}

if (isEntry(import.meta.url)) run(main);
