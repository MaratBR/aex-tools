// Terminal views. A program aex runs in a pseudo-console (a custom tool's script) shows in its run
// as a terminal (xterm.js, in vendor/): keys typed there go to it (TermInput), its screen comes as
// "term-output" events until "term-end". Backend: terminal.go.

// xterm.js's Terminal: the plain name is ansi.js's output renderer.
const XTerm = window.Terminal;
const terminals = new Map(); // id -> {term, fit, box, live}
const termRows = 24;

function termTheme() {
  const css = getComputedStyle(document.documentElement);
  const v = name => css.getPropertyValue(name).trim();
  return {
    background: v('--surface'), foreground: v('--ink'),
    cursor: v('--accent'), cursorAccent: v('--surface'),
    selectionBackground: v('--fill'),
  };
}

// fitCols fits the terminal's columns to its box; rows stay as they are.
function fitCols(t) {
  const d = t.fit.proposeDimensions();
  if (d && d.cols > 0 && d.cols !== t.term.cols) t.term.resize(d.cols, t.term.rows);
}

function onTerminal({ id }) {
  closeOut();
  if (!run) {
    startRun(['aex'], []);
    run.el.classList.add('orphan');
  }
  const box = el('div', 'term');
  run.body.appendChild(box);
  const css = getComputedStyle(document.documentElement);
  const term = new XTerm({
    rows: termRows, cols: 80, scrollback: 5000, cursorBlink: true,
    fontFamily: css.getPropertyValue('--mono').trim(), fontSize: 12.5, lineHeight: 1.15,
    theme: termTheme(),
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(box);
  const t = { term, fit, box, live: true };
  terminals.set(id, t);
  term.onData(d => { if (t.live) api().TermInput(id, d).catch(() => {}); });
  term.onResize(({ cols, rows }) => { if (t.live) api().TermResize(id, cols, rows).catch(() => {}); });
  fitCols(t);
  api().TermResize(id, term.cols, term.rows).catch(() => {});
  setStatus('running', 'Running · type in the terminal');
  term.focus();
  toBottom();
}

function onTermOutput({ id, data }) {
  const t = terminals.get(id);
  if (!t) return;
  const stick = nearBottom();
  t.term.write(Uint8Array.from(atob(data), c => c.charCodeAt(0)), () => { if (stick) toBottom(); });
}

// onTermEnd leaves the terminal as the program left it, without a cursor, and shrinks it to the
// lines it used when they fit.
function onTermEnd({ id }) {
  const t = terminals.get(id);
  if (!t) return;
  t.live = false;
  t.term.options.disableStdin = true;
  t.term.options.cursorBlink = false;
  t.term.write('\x1b[?25l', () => {
    const b = t.term.buffer.active;
    if (b.length > t.term.rows) return;
    let used = 0;
    for (let i = b.length - 1; i >= 0; i--) {
      if (b.getLine(i)?.translateToString(true).trim()) { used = i + 1; break; }
    }
    // Fewer rows than the cursor's line would scroll the top line away.
    t.term.resize(t.term.cols, Math.min(t.term.rows, Math.max(used, b.baseY + b.cursorY + 1, 1)));
  });
  t.term.blur();
  if (run) setStatus('running', 'Running');
}

// liveTerminal is the terminal of the program running now, if any.
function liveTerminal() {
  for (const [id, t] of terminals) if (t.live) return [id, t];
  return null;
}

// Terminals of runs cleared away are let go.
function pruneTerminals() {
  for (const [id, t] of terminals) {
    if (!t.box.isConnected) { t.term.dispose(); terminals.delete(id); }
  }
}

window.addEventListener('resize', () => {
  pruneTerminals();
  terminals.forEach(t => { if (t.live) fitCols(t); });
});
new MutationObserver(() => {
  pruneTerminals();
  terminals.forEach(t => (t.term.options.theme = termTheme()));
}).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });

window.runtime.EventsOn('terminal', onTerminal);
window.runtime.EventsOn('term-output', onTermOutput);
window.runtime.EventsOn('term-end', onTermEnd);
// A file dropped while no question takes a path is typed into the running terminal, as it is (a
// script mostly reads it with Read-Host, where quotes would be part of it).
window.runtime.OnFileDrop((x, y, paths) => {
  if (dropHandler || !paths || !paths.length) return;
  const live = liveTerminal();
  if (!live) return;
  api().TermInput(live[0], paths.join(' ')).catch(() => {});
  live[1].term.focus();
}, false);
