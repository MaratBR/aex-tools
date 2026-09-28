import readline from 'node:readline';
import { err as color } from './color.js';

// Prompts go to stderr so stdout stays clean.

export function assertInteractive(what) {
  if (!process.stdin.isTTY) throw new Error(`${what} needs an interactive terminal`);
}

// One reader for the whole process so lines typed/piped ahead of a prompt are not lost.
// Paused between prompts so it does not keep the process alive.
let rl = null;
let lines = null;

function closeLineReader() {
  rl?.close();
  rl = lines = null;
}

export async function ask(question) {
  if (!rl) {
    rl = readline.createInterface({ input: process.stdin, terminal: false });
    lines = rl[Symbol.asyncIterator]();
  }
  process.stderr.write(question);
  rl.resume();
  const { value, done } = await lines.next();
  rl?.pause();
  if (done) throw new Error('Input closed');
  return value.trim();
}

export async function confirm(question, defaultYes = false) {
  const a = (await ask(`${question} ${color.dim(defaultYes ? '[Y/n]' : '[y/N]')} `)).toLowerCase();
  return a ? a === 'y' || a === 'yes' : defaultYes;
}

// Reads a line from the TTY without echoing it.
export function promptSecret(question) {
  closeLineReader();
  return new Promise((resolve, reject) => {
    const { stdin, stderr } = process;
    stderr.write(question);
    stdin.setRawMode(true);
    stdin.resume();
    stdin.setEncoding('utf8');

    let input = '';
    const done = (err) => {
      stdin.setRawMode(false);
      stdin.pause();
      stdin.off('data', onData);
      stderr.write('\n');
      err ? reject(err) : resolve(input);
    };
    const onData = (chunk) => {
      for (const ch of chunk) {
        if (ch === '\r' || ch === '\n') return done();
        if (ch === '\u0003') return done(new Error('Cancelled'));
        if (ch === '\u0008' || ch === '\u007f') input = input.slice(0, -1);
        else if (ch >= ' ') input += ch;
      }
    };
    stdin.on('data', onData);
  });
}
