import { isEntry, printError, run } from '../lib/cli.js';
import { DATA_DIR, appRoot, hoursPerDay, isExe } from '../lib/config.js';
import { tzLabel } from '../lib/dates.js';
import { configEnvFile } from '../lib/env.js';
import { ask } from '../lib/prompt.js';
import { out } from '../lib/color.js';
import { createAextClient } from '../lib/aext.js';
import { leaveScreen, readKey, redraw, select } from '../lib/tui.js';
import * as worklogSync from './worklog-sync.js';
import * as quota from './quota.js';
import * as configure from './configure.js';
import * as dataFolder from './data-folder.js';

const TOOLS = [
  { name: 'worklog-sync', ...worklogSync },
  { name: 'quota', ...quota },
  { name: 'configure', ...configure },
  { name: 'data-folder', ...dataFolder },
];

const HELP = `Usage: aex [--data-dir <dir>] [--plain] [<tool> [args...]]

Without a tool, opens a menu to pick and run tools until you quit: an arrow-key
terminal UI, or a numbered list with --plain (or AEX_TUI=0, or when not on a terminal).
With a tool name, runs that tool once with the given args (try "aex <tool> --help").

Tools:
${TOOLS.map((t) => `  ${t.name.padEnd(14)}${t.summary}`).join('\n')}

Data folder (app settings .env.config, AEXT session, output), set with --data-dir <dir> or AEX_DATA_DIR:
  ${DATA_DIR}
.env and .env.private (fallbacks) are read from:
  ${appRoot}`;

const findTool = (input) => TOOLS[Number(input) - 1] ?? TOOLS.find((t) => t.name === input);

// Splits a line into args, keeping "quoted parts" together.
const splitArgs = (line) => (line.match(/"[^"]*"|\S+/g) ?? []).map((a) => a.replace(/^"(.*)"$/, '$1'));

// AEXT login shown in the menu header. Checked at start and after each tool (which may log in or out),
// without ever prompting for a login.
let login = { state: 'checking' };
function checkLogin() {
  login = { state: 'checking' };
  return Promise.resolve()
    .then(() => createAextClient().whoAmI())
    .then(
      (me) => (login = me ? { state: 'in', me } : { state: 'out' }),
      (e) => (login = { state: 'error', message: e.name === 'TimeoutError' ? 'not reachable (timed out)' : e.message }),
    );
}

// Header lines as [text, style] segments.
function infoLines() {
  const { state, me, message } = login;
  const who = {
    checking: [['AEXT: checking login…', out.dim]],
    in: [['Logged in as ', out.dim], [me?.display_name || me?.user_name || '', out.bold], [` <${me?.email}>`, out.dim]],
    out: [['Not logged in to AEXT', out.yellow], [' (tools log in when needed)', out.dim]],
    error: [['Login check failed: ', out.dim], [message ?? '', out.yellow]],
  }[state];
  return [
    who,
    [['Settings: ', out.dim], [`${hoursPerDay()} h/day, ${tzLabel()}`], [' (configure to change)', out.dim]],
    [['Settings file: ', out.dim], [configEnvFile]],
    [['Data folder: ', out.dim], [DATA_DIR]],
  ];
}

const plainLine = (segments) => segments.map(([text, style = String]) => style(text)).join('');

function printMenu() {
  console.log(out.bold('aex tools'));
  for (const line of infoLines()) console.log(plainLine(line));
  TOOLS.forEach((t, i) => console.log(`  ${out.cyan(`${i + 1})`)} ${t.name.padEnd(14)}${out.dim(t.summary)}`));
  console.log(`  ${out.cyan('q)')} quit`);
  console.log(out.dim('  Args may follow the choice, e.g. "1 --range yesterday" or "quota --help".'));
}

async function plainMenu() {
  for (;;) {
    await checkLogin();
    printMenu();
    const [choice, ...args] = splitArgs(await ask('> '));
    if (!choice) continue;
    if (['q', 'quit', 'exit'].includes(choice.toLowerCase())) return;
    const tool = findTool(choice);
    if (!tool) {
      console.log(`Unknown choice: ${choice}\n`);
      continue;
    }
    console.log();
    await runTool(tool, args);
    console.log();
  }
}

async function runTool(tool, args) {
  console.log(out.bold(`▶ ${[tool.name, ...args].join(' ')}`));
  try {
    await tool.main(args);
    return out.green(`● ${tool.name} finished`);
  } catch (err) {
    printError(err);
    return out.red(`● ${tool.name} failed: ${err.message}`);
  }
}

const QUIT = { name: 'quit', summary: 'Close aex', key: 'q' };

async function tuiMenu() {
  const items = [...TOOLS, QUIT];
  let index = 0;
  let status = '';
  checkLogin().then(redraw);
  for (;;) {
    const pick = await select({
      title: 'aex tools',
      info: infoLines,
      items,
      index,
      status,
      hint: '↑↓ move · Enter run · a run with args · q quit',
      extraKeys: { a: 'args' },
    });
    if (pick.index === null || items[pick.index] === QUIT) return;
    index = pick.index;
    const tool = items[index];

    leaveScreen();
    const args = pick.action === 'args' ? splitArgs(await ask(`${tool.name} args (try --help): `)) : [];
    status = await runTool(tool, args);
    process.stderr.write(out.dim('\nPress any key to return to the menu...'));
    checkLogin().then(redraw);
    await readKey();
  }
}

const useTui = () => process.stdin.isTTY && process.stdout.isTTY && process.env.AEX_TUI !== '0';

export async function main(argv) {
  const plain = argv[0] === '--plain';
  const [first, ...rest] = plain ? argv.slice(1) : argv;
  if (first === '--help' || first === '-h') {
    console.log(HELP);
    return;
  }
  if (!first) return plain || !useTui() ? plainMenu() : tuiMenu();
  const tool = findTool(first);
  if (!tool) throw new Error(`Unknown tool: ${first} (see aex --help)`);
  await tool.main(rest);
}

if (isExe || isEntry(import.meta.url)) run(main);
