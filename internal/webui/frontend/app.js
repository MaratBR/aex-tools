// A feed of tool runs. Each run is a block: its command and status, then what the tool prints,
// with its questions asked (and answered) in place. Backend: window.go.webui.App (webui.go),
// events from window.runtime.
const api = () => window.go.webui.App;
const $ = id => document.getElementById(id);
const feed = $('feed'), scroller = $('scroll'), input = $('input'), commandForm = $('command');

let tools = [];
let run = null;     // the run in progress: {el, body, status, out, term, started}
let keyHandler = null;

// Helpers ---------------------------------------------------------------------------------------

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

function button(label, cls, onclick) {
  const b = el('button', cls, label);
  b.type = 'button';
  b.onclick = onclick;
  return b;
}

// splitArgs matches the terminal: spaces split, "quoted parts" stay together.
function splitArgs(line) {
  return (line.match(/"[^"]*"|\S+/g) || []).map(a => (a.length >= 2 && a[0] === '"' && a.endsWith('"') ? a.slice(1, -1) : a));
}

// resolve takes a command line to the tool path it names and the args after it.
function resolve(line) {
  const words = splitArgs(line);
  const path = [];
  let list = tools;
  while (words.length && list) {
    const t = list.find(t => t.name === words[0]);
    if (!t) break;
    path.push(words.shift());
    list = t.sub;
  }
  return { path, args: words };
}

function plainText(ansi) {
  const s = el('span');
  s.appendChild(ansiToFragment(ansi));
  return s.textContent.trim();
}

// optionName is an option label without its dim part (the summary tools put after a name).
function optionName(ansi) {
  const s = el('span');
  s.appendChild(ansiToFragment(ansi));
  s.querySelectorAll('.d').forEach(d => d.remove());
  return s.textContent.trim() || plainText(ansi);
}

const nearBottom = () => scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 80;
const toBottom = () => (scroller.scrollTop = scroller.scrollHeight);

function elapsed(ms) {
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
}

// Runs ------------------------------------------------------------------------------------------

function setStatus(state, text) {
  run.el.dataset.state = state;
  run.status.textContent = text;
}

function startRun(path, args) {
  $('welcome').hidden = true;
  $('clear').hidden = false;
  const block = el('article', 'run');
  const head = el('header', 'run-head');
  const cmd = el('span', 'cmd');
  cmd.append(el('span', 'tool', path.join(' ')));
  if (args.length) cmd.append(' ', el('span', 'args', args.join(' ')));
  const status = el('span', 'status');
  head.append(cmd, status);
  const body = el('div', 'run-body');
  block.append(head, body);
  feed.appendChild(block);
  run = { el: block, body, status, out: null, term: null, started: Date.now() };
  setStatus('running', 'Running');
  document.body.classList.add('running');
  updateCommand();
  toBottom();
}

// outBlock is where output goes: the run's last output block, or a new one.
function outBlock() {
  if (!run) {
    // Output outside a run (a warning at start): its own block.
    startRun(['aex'], []);
    run.el.classList.add('orphan');
  }
  if (!run.out) {
    run.out = el('div', 'out');
    run.body.appendChild(run.out);
    run.term = new Terminal(run.out);
  }
  return run.term;
}

// closeOut ends the current output block, dropping blank lines around it.
function closeOut() {
  if (!run || !run.out) return;
  const lines = run.out.children;
  const blank = l => !l.textContent.trim();
  while (lines.length && blank(lines[0])) lines[0].remove();
  while (lines.length && blank(lines[lines.length - 1])) lines[lines.length - 1].remove();
  if (!lines.length) run.out.remove();
  run.out = run.term = null;
}

function onOutput(s) {
  const stick = nearBottom();
  outBlock().write(s);
  if (stick) toBottom();
}

async function runLine(line) {
  line = line.trim();
  if (!line || run) return;
  const { path, args } = resolve(line);
  if (!path.length) {
    showCommandError(`There is no tool called "${splitArgs(line)[0]}".`);
    return;
  }
  startRun(path, args);
  try {
    await api().Run(path, args);
  } catch (e) {
    finished({ status: String(e), ok: false });
  }
}

function finished({ status, ok, quiet }) {
  if (!run) return;
  closeOut();
  closeQuestion();
  if (quiet) {
    // The start-up questions: keep them only if they said something.
    if (!run.body.children.length) run.el.remove();
    else setStatus('done', '');
    if (!feed.children.length) { $('welcome').hidden = false; $('clear').hidden = true; }
    run = null;
    document.body.classList.remove('running');
    updateCommand();
    return;
  }
  const took = elapsed(Date.now() - run.started);
  setStatus(ok ? 'done' : 'failed', (ok ? 'Done' : 'Failed') + ' in ' + took);
  // A failure's error is already in the output unless the tool printed nothing.
  if (!ok && !run.body.querySelector('.out')) {
    run.body.appendChild(el('div', 'fail-note', status.replace(/^✖\s*/, '')));
  }
  if (!run.body.children.length) run.el.classList.add('empty');
  run = null;
  document.body.classList.remove('running');
  updateCommand();
  if (nearBottom()) toBottom();
  refresh();
}

// Questions -------------------------------------------------------------------------------------

function closeQuestion() {
  if (keyHandler) document.removeEventListener('keydown', keyHandler, true);
  keyHandler = null;
  document.querySelectorAll('.question.open').forEach(q => {
    q.classList.remove('open');
    q.querySelectorAll('button, input').forEach(b => (b.disabled = true));
  });
}

// answer sends the answer and leaves it in the question: shown is how it reads there.
function answer(p, q, value, shown, cancelled = false) {
  closeQuestion();
  const a = el('div', 'answer' + (cancelled ? ' cancelled' : ''), shown);
  q.querySelector('.controls')?.replaceWith(a);
  if (run) setStatus('running', 'Running');
  api().Answer(p.id, value, cancelled);
}

function onPrompt(p) {
  closeOut();
  if (!run) {
    startRun(['aex'], []);
    run.el.classList.add('orphan');
  }
  setStatus('waiting', 'Needs your answer');
  // Asked again after a rejected answer: this replaces the question it asked before.
  const last = run.body.lastElementChild;
  if (p.error && last && last.classList.contains('question') && last.dataset.title === p.title) last.remove();
  const q = el('section', 'question open');
  q.dataset.title = p.title;
  const cancel = () => answer(p, q, '', 'Cancelled', true);
  const title = el('div', 'q-title');
  title.appendChild(ansiToFragment(p.title));
  q.appendChild(title);
  if (p.description) q.appendChild(el('div', 'q-desc', p.description));
  const controls = el('div', 'controls');
  q.appendChild(controls);
  run.body.appendChild(q);

  if (p.kind === 'input') {
    const form = el('form', 'q-input');
    const field = el('input');
    field.type = p.secret ? 'password' : 'text';
    field.placeholder = p.placeholder || '';
    field.value = p.value || '';
    field.spellcheck = false;
    field.autocomplete = 'off';
    const ok = el('button', 'btn primary', 'Send');
    ok.type = 'submit';
    form.append(field, ok, button('Cancel', 'btn quiet', cancel));
    controls.appendChild(form);
    if (p.error) controls.appendChild(el('div', 'q-error', p.error.charAt(0).toUpperCase() + p.error.slice(1) + '.'));
    if (p.describe) {
      const live = el('div', 'q-live');
      controls.appendChild(live);
      let seq = 0;
      const describe = async () => {
        const n = ++seq;
        const text = await api().Describe(p.id, field.value);
        if (n !== seq) return;
        live.textContent = '';
        live.appendChild(ansiToFragment(text || ''));
      };
      field.addEventListener('input', describe);
      describe();
    }
    form.onsubmit = e => {
      e.preventDefault();
      const v = field.value;
      answer(p, q, v, p.secret ? (v ? '••••••••' : 'Left empty') : v.trim() || 'Left empty');
    };
    field.addEventListener('keydown', e => { if (e.key === 'Escape') { e.preventDefault(); cancel(); } });
    field.focus();
  } else if (p.kind === 'confirm') {
    const pick = (value, label, b) => { b.classList.add('picked'); answer(p, q, value, label); };
    const yes = button('Yes', 'btn' + (p.defaultYes ? ' primary' : ''), () => pick('yes', 'Yes', yes));
    const no = button('No', 'btn' + (p.defaultYes ? '' : ' primary'), () => pick('no', 'No', no));
    controls.append(yes, no, button('Cancel', 'btn quiet', cancel));
    controls.classList.add('row');
    keyHandler = e => {
      const k = e.key.toLowerCase();
      if (k === 'y') yes.click();
      else if (k === 'n') no.click();
      else if (k === 'escape') cancel();
      else return;
      e.preventDefault();
    };
    (p.defaultYes ? yes : no).focus();
  } else if (p.kind === 'choose') {
    const list = el('div', 'options');
    p.options.forEach((o, i) => {
      const b = button('', 'option', () => answer(p, q, o.value, optionName(o.label)));
      b.append(el('kbd', null, String(i + 1)));
      const label = el('span', 'label');
      label.appendChild(ansiToFragment(o.label));
      b.append(label);
      list.appendChild(b);
    });
    controls.append(list, button('Cancel', 'btn quiet', cancel));
    keyHandler = e => {
      const n = Number(e.key);
      if (n >= 1 && n <= p.options.length) list.children[n - 1].click();
      else if (e.key === 'Escape') cancel();
      else return;
      e.preventDefault();
    };
    list.firstChild.focus();
  } else if (p.kind === 'key') {
    const b = button('Continue', 'btn primary', () => answer(p, q, '', 'Continued'));
    controls.appendChild(b);
    b.focus();
  }
  if (keyHandler) document.addEventListener('keydown', keyHandler, true);
  toBottom();
}

// Command line ----------------------------------------------------------------------------------

function updateCommand() {
  input.disabled = !!run;
  input.placeholder = run ? '' : 'Or type a command, like quota --verbose';
  commandForm.classList.toggle('filled', !!input.value.trim());
  hideSuggest();
}

function showCommandError(text) {
  const e = $('command-error');
  e.textContent = text;
  e.hidden = false;
}

commandForm.onsubmit = e => {
  e.preventDefault();
  if (suggestIndex >= 0) { complete(suggestIndex); return; }
  const line = input.value;
  input.value = '';
  runLine(line);
};

// Tool name suggestions while typing a command.
let suggestions = [], suggestIndex = -1;

function suggest() {
  if (run) return hideSuggest();
  const words = input.value.split(/\s+/);
  const last = words.pop();
  let list = tools;
  for (const w of words) {
    const t = (list || []).find(t => t.name === w);
    if (!t) return hideSuggest();
    list = t.sub;
  }
  if (!list || (!last && !words.length)) return hideSuggest();
  suggestions = list.filter(t => t.name.startsWith(last) && t.name !== last).map(t => ({ t, prefix: words.join(' ') }));
  if (!suggestions.length) return hideSuggest();
  suggestIndex = -1;
  const ul = $('suggest');
  ul.textContent = '';
  suggestions.forEach((s, i) => {
    const li = el('li');
    li.setAttribute('role', 'option');
    li.append(el('span', 'name', s.t.name), el('span', 'summary', s.t.summary));
    li.onmousedown = e => { e.preventDefault(); complete(i); };
    ul.appendChild(li);
  });
  ul.hidden = false;
}

function hideSuggest() { $('suggest').hidden = true; suggestions = []; suggestIndex = -1; }

function highlight(i) {
  suggestIndex = i;
  [...$('suggest').children].forEach((li, j) => li.classList.toggle('active', j === i));
}

function complete(i) {
  const s = suggestions[i];
  input.value = (s.prefix ? s.prefix + ' ' : '') + s.t.name + ' ';
  input.focus();
  suggest();
}

input.addEventListener('input', () => {
  $('command-error').hidden = true;
  commandForm.classList.toggle('filled', !!input.value.trim());
  suggest();
});
input.addEventListener('keydown', e => {
  if ($('suggest').hidden) return;
  const n = suggestions.length;
  if (e.key === 'ArrowDown') { e.preventDefault(); highlight((suggestIndex + 1) % n); }
  else if (e.key === 'ArrowUp') { e.preventDefault(); highlight((suggestIndex - 1 + n) % n); }
  else if (e.key === 'Tab') { e.preventDefault(); complete(Math.max(suggestIndex, 0)); }
  else if (e.key === 'Escape') hideSuggest();
});
input.addEventListener('blur', () => setTimeout(hideSuggest, 100));

// Sidebar and welcome ---------------------------------------------------------------------------

function renderTools() {
  const nav = $('tools');
  nav.textContent = '';
  const add = (list, parent, path, depth) => {
    for (const t of list) {
      const p = [...path, t.name];
      const item = button('', 'tool' + (t.sub ? ' group' : ''));
      item.style.setProperty('--depth', depth);
      item.title = t.summary;
      item.append(el('span', 'name', t.name), el('span', 'summary', t.summary));
      parent.appendChild(item);
      if (t.sub) {
        const box = el('div', 'sub');
        box.hidden = true;
        parent.appendChild(box);
        add(t.sub, box, p, depth + 1);
        item.setAttribute('aria-expanded', 'false');
        item.onclick = () => {
          box.hidden = !box.hidden;
          item.setAttribute('aria-expanded', String(!box.hidden));
        };
      } else {
        item.onclick = () => runLine(p.join(' '));
      }
    }
  };
  add(tools, nav, [], 0);

  const starters = $('starters');
  starters.textContent = '';
  for (const t of tools) {
    const b = button('', 'starter', () => runLine(t.name));
    b.append(el('span', 'name', t.name), el('span', 'summary', t.summary));
    starters.appendChild(b);
  }
}

function renderInfo(lines) {
  const info = $('info');
  info.textContent = '';
  for (const l of lines) {
    const row = el('div', 'info-row' + (l.state ? ' login ' + l.state : ''));
    row.appendChild(el('span', 'label', l.label));
    const text = el('span', 'text');
    for (const s of l.segments || []) text.appendChild(el('span', s.kind, s.text));
    text.title = text.textContent;
    row.appendChild(text);
    info.appendChild(row);
  }
}

async function refresh() {
  tools = await api().Tools();
  renderTools();
  renderInfo(await api().Header(false));
  renderInfo(await api().Header(true));
}

$('clear').onclick = () => {
  if (run) return;
  feed.textContent = '';
  $('welcome').hidden = false;
  $('clear').hidden = true;
};

// Wiring ----------------------------------------------------------------------------------------

window.runtime.EventsOn('output', onOutput);
window.runtime.EventsOn('prompt', onPrompt);
window.runtime.EventsOn('finished', finished);
updateCommand();
refresh().then(async () => {
  if (run) return;
  // Its block goes up first: its output may arrive before Ready returns.
  startRun(['aex'], []);
  run.el.classList.add('orphan');
  if (!(await api().Ready())) finished({ quiet: true, ok: true });
});
