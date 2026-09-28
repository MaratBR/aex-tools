import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { run } from '../lib/cli.js';
import { REPO_ZIP_DIR, repoRoot } from '../lib/config.js';
import { fileTimestamp } from '../lib/dates.js';

const HELP = `Usage: zip-repo

Zips the files committed at HEAD into output/repo-zip/<repo>-<commit>-<timestamp>.zip.
Uncommitted, staged, untracked and gitignored files are left out (uses git archive).`;

run(async (argv) => {
  if (argv.includes('--help') || argv.includes('-h')) {
    console.log(HELP);
    return;
  }
  const git = (...args) => execFileSync('git', args, { cwd: repoRoot, encoding: 'utf8' }).trim();
  const commit = git('rev-parse', '--short', 'HEAD');
  const name = path.basename(repoRoot);
  fs.mkdirSync(REPO_ZIP_DIR, { recursive: true });
  const zipFile = path.join(REPO_ZIP_DIR, `${name}-${commit}-${fileTimestamp()}.zip`);
  git('archive', '--format=zip', `--prefix=${name}/`, '-o', zipFile, 'HEAD');
  if (git('status', '--porcelain', '--untracked-files=no')) {
    console.log('note: uncommitted changes are not included');
  }
  console.log(zipFile);
});
