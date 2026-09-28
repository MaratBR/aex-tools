// Minimal ANSI styling. Enabled per stream when it is a TTY; honors NO_COLOR and FORCE_COLOR.
const STYLES = {
  bold: [1, 22],
  dim: [2, 22],
  red: [31, 39],
  green: [32, 39],
  yellow: [33, 39],
  cyan: [36, 39],
};

function enabled(stream) {
  const force = process.env.FORCE_COLOR;
  if (force !== undefined) return force !== '0' && force !== 'false';
  if (process.env.NO_COLOR) return false;
  return Boolean(stream.isTTY) && process.env.TERM !== 'dumb';
}

function palette(stream) {
  const on = enabled(stream);
  return Object.fromEntries(
    Object.entries(STYLES).map(([name, [open, close]]) => [
      name,
      (text) => (on ? `\x1b[${open}m${text}\x1b[${close}m` : String(text)),
    ]),
  );
}

// `out` for text written to stdout, `err` for stderr.
export const out = palette(process.stdout);
export const err = palette(process.stderr);
