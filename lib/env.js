import { existsSync, readFileSync } from 'node:fs';
import { appendFile } from 'node:fs/promises';
import path from 'node:path';
import { repoRoot } from './config.js';
import { assertInteractive, promptSecret, confirm } from './prompt.js';
import { err as color } from './color.js';

export const privateEnvFile = path.join(repoRoot, '.env.private');

// loadEnvFile never overwrites a set var, so first load wins: real env > .env.private > .env.
for (const file of [privateEnvFile, path.join(repoRoot, '.env')]) {
  if (existsSync(file)) process.loadEnvFile(file);
}

export function requireEnv(name) {
  const value = process.env[name];
  if (!value) throw new Error(`Missing required env var ${name} (set it in .env)`);
  return value;
}

// Returns a secret from env; if missing, prompts for it and offers to save it to .env.private.
export async function requireSecret(name, hint) {
  if (process.env[name]) return process.env[name];
  assertInteractive(`Missing ${name} (set it in .env.private or the environment); prompting for it`);

  if (hint) console.error(color.yellow(hint));
  const value = (await promptSecret(`${name}: `)).trim();
  if (!value) throw new Error(`${name} is required`);
  process.env[name] = value;

  if (await confirm(`Save ${name} to .env.private?`, true)) {
    const existing = existsSync(privateEnvFile) ? readFileSync(privateEnvFile, 'utf8') : '';
    const sep = existing && !existing.endsWith('\n') ? '\n' : '';
    await appendFile(privateEnvFile, `${sep}${name}=${value}\n`, 'utf8');
    console.error(color.green(`Saved to ${privateEnvFile}`));
  }
  return value;
}
