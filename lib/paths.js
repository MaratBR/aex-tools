// Where things live. Kept free of settings so lib/env.js can load settings from these paths.
import os from 'node:os';
import path from 'node:path';
import { isSea } from 'node:sea';
import { fileURLToPath } from 'node:url';

// True when running as the built single-file executable (dist/aex.exe).
export const isExe = isSea();

// Where .env and .env.private live: the repo, or the folder holding the exe.
export const appRoot = isExe
  ? path.dirname(process.execPath)
  : path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// Global option every entry point accepts: --data-dir <dir> (or --data-dir=<dir>).
// Returns the value (last one wins) and the args without it; run() in lib/cli.js strips it.
export function takeGlobalArgs(argv) {
  const rest = [];
  let dataDir;
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '--data-dir') {
      dataDir = argv[++i];
      if (!dataDir) throw new Error('--data-dir needs a folder');
    } else if (arg.startsWith('--data-dir=')) dataDir = arg.slice('--data-dir='.length);
    else rest.push(arg);
  }
  return { dataDir, rest };
}

// Per-user app data folder (like other apps' settings): %APPDATA%\aex on Windows.
function defaultDataDir() {
  const home = os.homedir();
  if (process.platform === 'win32') return path.join(process.env.APPDATA || path.join(home, 'AppData', 'Roaming'), 'aex');
  if (process.platform === 'darwin') return path.join(home, 'Library', 'Application Support', 'aex');
  return path.join(process.env.XDG_CONFIG_HOME || path.join(home, '.config'), 'aex');
}

// A bad --data-dir is reported by run() instead of crashing on import here.
function dataDirArg() {
  try {
    return takeGlobalArgs(process.argv.slice(2)).dataDir;
  } catch {
    return undefined;
  }
}

// Where app settings (.env.config), the AEXT session and output files live: --data-dir, else
// AEX_DATA_DIR (real environment only: app settings live here), else the per-user default.
export const DATA_DIR = path.resolve(dataDirArg() || process.env.AEX_DATA_DIR || defaultDataDir());

export const JIRA_EXPORT_DIR = path.join(DATA_DIR, 'output', 'jira-export');
export const REPO_ZIP_DIR = path.join(DATA_DIR, 'output', 'repo-zip');
export const AEXT_SESSION_FILE = process.env.AEXT_SESSION_FILE || path.join(DATA_DIR, 'session.txt');
