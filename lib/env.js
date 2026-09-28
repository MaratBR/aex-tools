import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { parseEnv } from 'node:util';
import { DATA_DIR, appRoot } from './paths.js';
import { ask, assertInteractive, promptSecret, confirm } from './prompt.js';
import { err as color } from './color.js';

export const sharedEnvFile = path.join(appRoot, '.env');
export const privateEnvFile = path.join(appRoot, '.env.private');
// App settings: auth settings are saved here (by configure and by the prompts below), and the
// APP_SETTINGS defaults are written here on first start.
export const configEnvFile = path.join(DATA_DIR, '.env.config');

// Auth settings. They belong in app settings; .env.private, .env and real env still work as fallbacks.
export const AUTH_SETTINGS = [
  { name: 'AEXT_EMAIL', hint: 'Email you log in to AEXT with' },
  { name: 'JIRA_EMAIL', hint: 'Email of your Atlassian (Jira) account' },
  {
    name: 'JIRA_TOKEN',
    hint: 'Jira Cloud API token: https://id.atlassian.com/manage-profile/security/api-tokens',
    secret: true,
  },
];

// Numeric settings with defaults. An invalid value falls back to the default, with a warning.
export const APP_SETTINGS = [
  {
    name: 'HOURS_PER_DAY',
    default: '8',
    hint: 'Working hours per day, used for the quota',
    rule: 'a number above 0, at most 24',
    valid: (n) => n > 0 && n <= 24,
  },
  {
    name: 'TZ_OFFSET_HOURS',
    default: '7',
    hint: 'Timezone as a UTC offset in hours: 7 = UTC+7, -5 = UTC-5, 5.5 = UTC+5:30',
    rule: 'a number from -12 to 14, in steps of 0.25',
    valid: (n) => n >= -12 && n <= 14 && Number.isInteger(n * 4),
  },
];

// Everything configure asks for.
export const SETTINGS = [...AUTH_SETTINGS, ...APP_SETTINGS];

const realEnv = { ...process.env };
const readEnvFile = (file) => (existsSync(file) ? parseEnv(readFileSync(file, 'utf8')) : {});

// Where settings come from, lowest priority first. App settings (.env.config) beat even real
// environment variables. The exe build embeds the repo's .env (see scripts/build-exe.js).
export function envLayers() {
  return [
    ...(typeof __EMBEDDED_ENV__ === 'string' ? [{ label: '.env built into the exe', vars: parseEnv(__EMBEDDED_ENV__) }] : []),
    { label: '.env', vars: readEnvFile(sharedEnvFile) },
    { label: '.env.private', vars: readEnvFile(privateEnvFile) },
    { label: 'environment variables', vars: realEnv },
    { label: 'app settings', vars: readEnvFile(configEnvFile) },
  ];
}

// Why `value` is not valid for setting `name`, or null when it is (or the setting has no rule).
export function settingProblem(name, value) {
  const def = APP_SETTINGS.find((s) => s.name === name);
  if (!def) return null;
  return String(value).trim() !== '' && def.valid(Number(value)) ? null : `${name} must be ${def.rule}, got "${value}"`;
}

const warned = new Set();
// Current value of an APP_SETTINGS entry, or its default if missing or invalid.
export function settingValue(name) {
  const def = APP_SETTINGS.find((s) => s.name === name);
  const value = process.env[name];
  if (value === undefined) return def.default;
  const problem = settingProblem(name, value);
  if (!problem) return value;
  if (!warned.has(name)) {
    warned.add(name);
    console.error(`${color.yellow('Warning:')} ${problem} (${configEnvFile}); using ${def.default}.`);
  }
  return def.default;
}

// Unquoted when parseEnv reads it back unchanged, else single-quoted (no escapes inside those).
function formatLine(name, value) {
  if (/^[\w.@+\-/:=]*$/.test(value)) return `${name}=${value}`;
  if (value.includes("'")) throw new Error(`${name} cannot contain a single quote`);
  return `${name}='${value}'`;
}

// Replaces or removes `name` in the file text, keeping other lines and comments.
function setLine(text, name, value) {
  const lines = text.split(/\r?\n/).filter((l, i, all) => l || i < all.length - 1);
  const at = lines.findIndex((l) => new RegExp(`^\\s*(export\\s+)?${name}\\s*=`).test(l));
  if (value === null) {
    if (at >= 0) lines.splice(at, 1);
  } else if (at >= 0) lines[at] = formatLine(name, value);
  else lines.push(formatLine(name, value));
  return lines.length ? `${lines.join('\n')}\n` : '';
}

const APP_SETTINGS_HEADER = '# aex app settings. Edit here or run "aex configure". Overrides .env, .env.private and env vars.\n';

// Writes { NAME: value | null (remove) } to app settings, keeping everything else in the file.
export function saveAppSettings(changes) {
  let text = existsSync(configEnvFile) ? readFileSync(configEnvFile, 'utf8') : APP_SETTINGS_HEADER;
  for (const [name, value] of Object.entries(changes)) text = setLine(text, name, value);
  mkdirSync(path.dirname(configEnvFile), { recursive: true });
  writeFileSync(configEnvFile, text, 'utf8');
}

Object.assign(process.env, ...envLayers().map((l) => l.vars));

// First start (or a setting added since): write APP_SETTINGS to app settings, so they are there to
// edit. Uses the value already in effect from a fallback source when valid, else the default.
function seedAppSettings() {
  const saved = readEnvFile(configEnvFile);
  const missing = APP_SETTINGS.filter((s) => saved[s.name] === undefined);
  if (!missing.length) return;
  const values = Object.fromEntries(
    missing.map((s) => {
      const current = process.env[s.name];
      return [s.name, current !== undefined && !settingProblem(s.name, current) ? current : s.default];
    }),
  );
  try {
    saveAppSettings(values);
    Object.assign(process.env, values);
  } catch (e) {
    console.error(`${color.yellow('Warning:')} could not write defaults to ${configEnvFile}: ${e.message}`);
  }
}
seedAppSettings();

export function requireEnv(name) {
  const value = process.env[name];
  if (!value) throw new Error(`Missing required env var ${name} (set it in .env)`);
  return value;
}

// Returns an auth setting (AUTH_SETTINGS); if missing, prompts for it and offers to save it to app settings.
export async function requireAuthSetting(name) {
  if (process.env[name]) return process.env[name];
  const { hint, secret } = AUTH_SETTINGS.find((s) => s.name === name);
  assertInteractive(`Missing ${name} (run configure to set it); prompting for it`);

  console.error(color.yellow(`${name} not set. ${hint}. (configure sets all of these at once.)`));
  const value = (await (secret ? promptSecret : ask)(`${name}: `)).trim();
  if (!value) throw new Error(`${name} is required`);
  process.env[name] = value;

  if (await confirm(`Save ${name} to app settings?`, true)) {
    saveAppSettings({ [name]: value });
    console.error(color.green(`Saved to ${configEnvFile}`));
  }
  return value;
}
