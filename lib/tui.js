// Full-screen arrow-key menu drawn with box characters. No dependencies: raw stdin + ANSI escapes.
import { out } from './color.js';
import { closeLineReader } from './prompt.js';

const write = (s) => process.stdout.write(s);

// The menu is drawn on the alternate screen, so tool output stays in the normal scrollback.
let onAltScreen = false;
export function enterScreen() {
  if (onAltScreen) return;
  onAltScreen = true;
  write('\x1b[?1049h\x1b[?25l');
}
export function leaveScreen() {
  if (!onAltScreen) return;
  onAltScreen = false;
  write('\x1b[?25h\x1b[?1049l');
}
process.on('exit', leaveScreen);

const KEYS = {
  '\x1b[A': 'up',
  '\x1bOA': 'up',
  k: 'up',
  '\x1b[B': 'down',
  '\x1bOB': 'down',
  j: 'down',
  '\x1b[H': 'home',
  '\x1b[F': 'end',
  '\r': 'enter',
  '\n': 'enter',
  '\x1b': 'escape',
  '\u0003': 'ctrl-c',
};

// Resolves with the next key pressed: a name from KEYS, or the raw character(s).
export function readKey() {
  closeLineReader();
  return new Promise((resolve) => {
    const { stdin } = process;
    stdin.setRawMode(true);
    stdin.setEncoding('utf8');
    stdin.resume();
    stdin.once('data', (chunk) => {
      stdin.setRawMode(false);
      stdin.pause();
      resolve(KEYS[chunk] ?? chunk);
    });
  });
}

// One boxed line from [text, style] segments, truncated with an ellipsis and padded to `inner`.
function row(inner, ...segments) {
  let used = 0;
  let line = '';
  for (const [text, style = String] of segments) {
    const room = inner - used;
    if (room <= 0) break;
    const part = text.length > room ? `${text.slice(0, room - 1)}…` : text;
    line += style(part);
    used += part.length;
  }
  return `${out.dim('│')}${line}${' '.repeat(inner - used)}${out.dim('│')}`;
}

const accent = (s) => out.bold(out.cyan(s));

function render({ title, info, items, index, hint, status }) {
  const inner = Math.max(30, Math.min((process.stdout.columns || 80) - 2, 72));
  const rule = (left, right) => out.dim(left + '─'.repeat(inner) + right);
  const lines = [
    rule('╭', '╮'),
    row(inner, ['  '], [title, accent]),
    ...(info?.() ?? []).map((segments) => row(inner, ['  '], ...segments)),
    rule('├', '┤'),
    row(inner),
    ...items.flatMap((item, i) => {
      const on = i === index;
      const key = item.key ?? String(i + 1);
      return [
        row(inner, [on ? ' ▌ ' : '   ', out.cyan], [key.padEnd(3), on ? out.cyan : out.dim], [item.name, on ? accent : String]),
        row(inner, ['      '], [item.summary ?? '', on ? String : out.dim]),
        row(inner),
      ];
    }),
    rule('╰', '╯'),
    `  ${out.dim(hint)}`,
    status ? `  ${status}` : '',
  ];
  // Overwrite in place (clear each line's tail, then below) instead of clearing first: no flicker.
  write(`\x1b[H${lines.map((l) => `${l}\x1b[K`).join('\n')}\x1b[J`);
}

// Redraws the open menu, e.g. after data its `info` shows has loaded.
let redrawMenu = null;
export const redraw = () => redrawMenu?.();

// Shows `items` ({ name, summary, key? }) until one is picked; `key` labels it and picks it
// (default: its number). `info` returns header lines, each a list of [text, style] segments.
// Returns { index, action }: action is 'run', or one of `extraKeys` values; null index on quit.
export async function select({ title, info, items, index = 0, hint, status, extraKeys = {} }) {
  enterScreen();
  const draw = () => render({ title, info, items, index, hint, status });
  redrawMenu = draw;
  process.stdout.on('resize', draw);
  try {
    for (;;) {
      draw();
      const key = await readKey();
      if (key === 'up') index = (index - 1 + items.length) % items.length;
      else if (key === 'down') index = (index + 1) % items.length;
      else if (key === 'home') index = 0;
      else if (key === 'end') index = items.length - 1;
      else if (key === 'enter') return { index, action: 'run' };
      else if (['q', 'escape', 'ctrl-c'].includes(key)) return { index: null, action: 'quit' };
      else if (extraKeys[key]) return { index, action: extraKeys[key] };
      else {
        const picked = items.findIndex((item, i) => (item.key ?? String(i + 1)) === key);
        if (picked >= 0) return { index: picked, action: 'run' };
      }
    }
  } finally {
    redrawMenu = null;
    process.stdout.off('resize', draw);
  }
}
