// The Composer page: composed tools (internal/composer), each a list of steps run in order, a step
// being an action: run a tool or a program, open a project in an IDE, show a message, check where
// the IP is, if / else, try steps until they work, return. Edited with blocks (actions and tools
// dragged from the palette into the script; an if or a try holds steps of its own) or as JSON;
// both edit the same draft. Edits are kept per tool while the window is open, until saved or discarded.
// Backend: composer.go (Composer, ComposerCheck, ComposerSave, ComposerDelete, ComposerBrowse).
const cmpNav = $('cmp-nav'), cmpMain = $('cmp-main');

let cmpState = null;       // ComposerState
const cmpDrafts = new Map(); // tool name as kept ('' for a new one): {draft, saved, text, mode, result}
let cmpOpen = null;        // the key of cmpDrafts shown, null for none
let cmpDragging = null;    // what is dragged: {from: path} or {make: step}
let cmpCheckTimer = 0;
let cmpPalQuery = '';      // the palette's tool search, kept while the editor is redrawn

// Kinds of action: their color, and the words the block starts with.
const cmpKinds = {
  tool: { word: 'Run tool', icon: 'M7 5l12 7-12 7z' },
  run: { word: 'Run program', icon: 'M4 5h16v14H4zM7 9l3 3-3 3M12 15h5' },
  'open-ide': { word: 'Open', icon: 'M8 6l-5 6 5 6M16 6l5 6-5 6M14 4l-4 16' },
  check: { word: 'Try until it works', icon: 'M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7' },
  if: { word: 'If', icon: 'M6 3v6a6 6 0 0 0 6 6h6M15 12l3 3-3 3M6 21v-4' },
  return: { word: 'Return', icon: 'M9 14l-5-5 5-5M4 9h10a6 6 0 0 1 0 12h-3' },
  message: { word: 'Show message', icon: 'M4 5h16v11H9l-5 4z' },
  delay: { word: 'Wait', icon: 'M12 7v5l3 2M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z' },
  home: { word: 'Switch to Home', icon: 'M3 10l9-7 9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1z' },
  minimize: { word: 'Minimize aex', icon: 'M5 19h14' },
  orient: { word: 'Orient windows', icon: 'M3 4h8v7H3zM13 4h8v7h-8zM3 13h18v7H3z' },
  'ip-check': { word: 'IP is', icon: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18' },
};

const cmpCur = () => (cmpOpen == null ? null : cmpDrafts.get(cmpOpen));
const cmpText = d => JSON.stringify(d.draft, null, 2);
// canonical is JSON text as one line, null when it is not JSON: what the editor compares.
const canonical = text => { try { return JSON.stringify(JSON.parse(text)); } catch { return null; } };
// cmpDirty: d differs from what is kept (a new one always does).
function cmpDirty(d) {
  if (d.saved === '') return true;
  const now = d.text != null ? canonical(d.text) : JSON.stringify(d.draft);
  return now == null || now !== canonical(d.saved);
}
const svgIcon = (path, cls = '') => `<svg class="stroke ${cls}" viewBox="0 0 24 24" aria-hidden="true"><path d="${path}"/></svg>`;

// Loading ---------------------------------------------------------------------------------------

async function onComposerPage() {
  try {
    cmpState = await api().Composer();
  } catch (e) {
    cmpMain.textContent = String(e);
    return;
  }
  // Drafts of tools no longer kept (removed elsewhere) go, unless changed here.
  for (const [k, d] of cmpDrafts) {
    if (k !== '' && !cmpState.tools.some(t => t.name === k) && !cmpDirty(d)) cmpDrafts.delete(k);
  }
  if (cmpOpen != null && !cmpDrafts.has(cmpOpen) && !cmpState.tools.some(t => t.name === cmpOpen)) cmpOpen = null;
  if (cmpOpen == null && cmpState.tools.length) cmpOpen = cmpState.tools[0].name;
  if (cmpOpen != null) cmpEnsure(cmpOpen);
  cmpRenderNav();
  cmpRender();
}

// cmpEnsure makes the draft of the tool kept as name, from what is kept, unless there is one.
function cmpEnsure(name) {
  if (cmpDrafts.has(name)) return cmpDrafts.get(name);
  const t = cmpState.tools.find(t => t.name === name);
  let draft = null, text = null;
  try { draft = JSON.parse(t.json); } catch {}
  if (!draft || typeof draft !== 'object') { draft = { name, steps: [] }; text = t.json; }
  const d = { draft, saved: t.json, text, mode: text != null ? 'json' : 'blocks', result: null };
  if (t.error) d.result = { error: t.error, problems: [] };
  cmpDrafts.set(name, d);
  cmpScheduleCheck(d);
  return d;
}

function cmpSelect(key) {
  cmpOpen = key;
  if (key !== '') cmpEnsure(key);
  cmpRenderNav();
  cmpRender();
}

function cmpNew(example = false) {
  let name = 'my-tool';
  for (let n = 2; cmpState.tools.some(t => t.name.toLowerCase() === name) || cmpState.lines.some(l => l.line === name); n++) name = 'my-tool-' + n;
  const draft = example ? cmpExample(name) : { name, summary: '', steps: [] };
  cmpDrafts.set('', { draft, saved: '', text: null, mode: 'blocks', result: null });
  cmpSelect('');
  cmpScheduleCheck(cmpCur());
  cmpMain.querySelector('.cmp-name')?.select();
}

// cmpExample is a composed tool to start from: the VPN on (until the IP is not in RU, starting
// the VPN and asking to turn it on), then the day's tools and the IDE (the first one found, with
// the project it opened last).
function cmpExample(name) {
  const ide = cmpState.ides[0];
  const steps = [{
    action: 'check', label: 'VPN on', critical: true, every: 10, timeout: 300, do: [{
      action: 'if', cond: { action: 'ip-check', want: ['RU'] },
      then: [
        { action: 'run', path: 'VPN.exe', detach: true, skipRunning: true },
        { action: 'message', title: 'VPN', text: 'Please enable the VPN' },
        { action: 'return', fail: true, message: 'the VPN is off' },
      ],
      else: [{ action: 'return' }],
    }],
  }];
  if (ide) steps.push({ action: 'open-ide', parallel: true, ide: ide.id, project: ide.projects[0]?.path || '' });
  steps.push({ action: 'tool', parallel: true, tool: 'quota' });
  return { name, summary: 'Start the day: VPN on, then my project', steps };
}

// cmpIDE is the IDE found on this device that id names ("vs": the newest Visual Studio), undefined
// when there is none.
const cmpIDE = id => cmpState.ides.find(i => i.id === id) || (id === 'vs' ? cmpState.ides.find(i => i.provider === 'Visual Studio') : undefined);

// Sidebar of the page: the composed tools ----------------------------------------------------

function cmpRenderNav() {
  cmpNav.textContent = '';
  const add = button('', 'btn small primary cmp-new', () => cmpNew());
  add.innerHTML = svgIcon('M12 5v14M5 12h14') + '<span>New tool</span>';
  cmpNav.append(add);
  const keys = cmpState.tools.map(t => t.name);
  if (cmpDrafts.has('')) keys.push('');
  for (const k of keys) {
    const t = cmpState.tools.find(t => t.name === k);
    const d = cmpDrafts.get(k);
    const b = button('', 'settings-link cmp-link', () => cmpSelect(k));
    b.setAttribute('aria-current', String(k === cmpOpen));
    const name = el('span', 'cmp-link-name', k === '' ? (d.draft.name || 'New tool') : k);
    const sub = el('span', 'cmp-link-sub', t ? (t.error ? 'Cannot be read' : `${t.steps} step${t.steps === 1 ? '' : 's'}`) : 'Not saved yet');
    if (t?.error) sub.classList.add('bad');
    b.append(name, sub);
    if (d && cmpDirty(d)) b.append(el('span', 'cmp-dirty', ''));
    b.title = t?.summary || '';
    cmpNav.append(b);
  }
  if (!keys.length) cmpNav.append(el('div', 'settings-nav-empty', 'No composed tools yet'));
}

// The editor -------------------------------------------------------------------------------------

function cmpRender() {
  cmpMain.textContent = '';
  const d = cmpCur();
  if (!d) { cmpRenderEmpty(); return; }

  const top = el('div', 'cmp-top');
  const ident = el('div', 'cmp-ident');
  const name = el('input', 'cmp-name');
  name.placeholder = 'tool-name';
  name.spellcheck = false;
  name.value = d.draft.name || '';
  name.setAttribute('aria-label', 'Name');
  name.oninput = () => { cmpSet(d.draft, 'name', name.value.trim()); cmpChanged(d, false); cmpSyncHat(); };
  const summary = el('input', 'cmp-summary');
  summary.placeholder = 'What it does (optional)';
  summary.value = d.draft.summary || '';
  summary.setAttribute('aria-label', 'Summary');
  summary.oninput = () => { cmpSet(d.draft, 'summary', summary.value); cmpChanged(d, false); };
  ident.append(name, summary);
  const acts = el('div', 'cmp-acts');
  if (cmpOpen !== '') {
    const del = button('Delete', 'btn small quiet cmp-del', () => {
      if (!del.classList.contains('armed')) {
        del.classList.add('armed');
        del.textContent = 'Delete for good?';
        setTimeout(() => { if (del.isConnected) { del.classList.remove('armed'); del.textContent = 'Delete'; } }, 3000);
        return;
      }
      cmpDelete();
    });
    acts.append(del);
  }
  const discard = button('Discard', 'btn small cmp-discard', () => cmpDiscard());
  const save = button('Save', 'btn small cmp-save', () => cmpSave());
  const runBtn = button('', 'btn small primary cmp-run', () => cmpRun());
  runBtn.innerHTML = svgIcon('M7 5l12 7-12 7z', 'fill') + '<span>Run</span>';
  acts.append(discard, save, runBtn);
  top.append(ident, acts);

  const bar = el('div', 'cmp-bar');
  const tabs = el('div', 'segmented cmp-tabs');
  tabs.setAttribute('role', 'tablist');
  for (const [mode, label] of [['blocks', 'Blocks'], ['json', 'JSON']]) {
    const t = button(label, 'cmp-tab', () => cmpMode(mode));
    t.setAttribute('role', 'tab');
    t.setAttribute('aria-selected', String(d.mode === mode));
    tabs.append(t);
  }
  const status = el('div', 'cmp-status');
  bar.append(tabs, status);

  const general = el('div', 'cmp-general');
  cmpMain.append(top, bar, general);
  if (d.mode === 'json') cmpMain.append(cmpJSONEditor(d));
  else cmpMain.append(cmpBlocksEditor(d));
  cmpShowResult(d);
}

function cmpRenderEmpty() {
  const box = el('div', 'cmp-empty');
  box.innerHTML = `<div class="cmp-empty-art" aria-hidden="true">
      <span class="blk-mini hat">When it runs</span>
      <span class="blk-mini" data-kind="ip-check">IP is in US</span>
      <span class="blk-mini" data-kind="open-ide">Open app.sln in Rider</span>
      <span class="blk-mini" data-kind="tool">Run tool quota</span>
    </div>
    <h2>Make a tool out of steps</h2>
    <p class="dim">A composed tool runs its steps in order: tools, programs, opening a project in an IDE,
      checking the VPN is on, or checking anything until it works. A step that fails is reported and
      the next one runs, unless it is critical. Steps marked parallel next to one another run together.</p>`;
  const row = el('div', 'cmp-empty-acts');
  row.append(button('New tool', 'btn primary', () => cmpNew()), button('Start from an example', 'btn', () => cmpNew(true)));
  box.append(row);
  cmpMain.append(box);
}

function cmpMode(mode) {
  const d = cmpCur();
  if (d.mode === mode) return;
  if (mode === 'blocks' && d.text != null) {
    let parsed;
    try { parsed = JSON.parse(d.text); } catch (e) {
      cmpFlash(`Fix the JSON first: ${e.message}`);
      return;
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed) || (parsed.steps != null && !Array.isArray(parsed.steps))) {
      cmpFlash('Fix the JSON first: it is {"name", "summary", "steps": [ … ]}');
      return;
    }
    d.draft = parsed;
    if (Array.isArray(parsed.steps)) cmpTidy(parsed.steps);
    d.text = null;
  }
  d.mode = mode;
  cmpRender();
}

function cmpFlash(text) {
  const s = cmpMain.querySelector('.cmp-status');
  if (!s) return;
  s.textContent = text;
  s.className = 'cmp-status bad';
}

// JSON editor ------------------------------------------------------------------------------------

function cmpJSONEditor(d) {
  const box = el('div', 'cmp-json');
  const area = el('textarea', 'cmp-json-text');
  area.spellcheck = false;
  area.value = d.text != null ? d.text : cmpText(d);
  area.setAttribute('aria-label', 'Composed tool as JSON');
  area.oninput = () => {
    d.text = area.value;
    try {
      const parsed = JSON.parse(area.value);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) d.draft = parsed;
    } catch {}
    cmpChanged(d, false);
  };
  area.onkeydown = e => {
    // Tab indents, as in a code editor (Esc then Tab leaves the box).
    if (e.key === 'Tab' && !e.shiftKey && !e.ctrlKey) {
      e.preventDefault();
      document.execCommand('insertText', false, '  ');
    } else if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault();
      cmpSave();
    }
  };
  const help = el('p', 'cmp-json-help');
  help.innerHTML = 'A step is <code>{"action": …, "label", "critical", "parallel", …its fields}</code>. Actions: ' +
    cmpState.kinds.map(k => `<code title="${k.summary.replace(/"/g, '&quot;')}">${k.id}</code>`).join(' ') +
    '. <code>aex composer actions</code> lists their fields.';
  box.append(area, help);
  return box;
}

// Blocks editor ----------------------------------------------------------------------------------

// Paths: a step is found by its path from the tool's steps: its index, and for a step inside
// another the part it is in, then its index there ('do', 'then', 'else' are lists), or 'cond' (an
// if's condition, one step). [2] is the third step, [2, 'then', 0] the first of its then.
function cmpWhere(d, path) {
  let list = d.draft.steps, key = path[0];
  for (let k = 1; k < path.length; k++) {
    const step = list[key];
    if (path[k] === 'cond') { list = step; key = 'cond'; continue; }
    list = cmpRead(step, path[k]);
    key = path[++k];
  }
  return { list, key };
}
const cmpStepAt = (d, path) => { const w = cmpWhere(d, path); return w.list?.[w.key]; };
// cmpRead is the list of steps in a step's part: [] when it has none (not added to it).
const cmpRead = (step, part) => (Array.isArray(step?.[part]) ? step[part] : step?.[part] ? [step[part]] : []);
// cmpListFor is the list of steps at prefix ([] for the tool's, [2, 'then'] for the then of the
// third step), added to its step when it has none, to put a step in.
function cmpListFor(d, prefix) {
  if (!prefix.length) return d.draft.steps;
  const owner = cmpStepAt(d, prefix.slice(0, -1));
  const part = prefix[prefix.length - 1];
  owner[part] = cmpRead(owner, part);
  return owner[part];
}
// cmpKey is a path as the backend numbers steps: "3", "3.then.1", "3.cond".
const cmpKey = path => path.map(p => (typeof p === 'number' ? p + 1 : p)).join('.');
// cmpInside: path is outer or inside it (a step cannot go into itself).
const cmpInside = (outer, path) => outer.length <= path.length && outer.every((p, i) => p === path[i]);

// cmpTidy writes the steps as the backend does: one step alone in a part as a list, empty parts
// left out.
function cmpTidy(steps) {
  for (const s of steps || []) {
    if (!s || typeof s !== 'object') continue;
    for (const part of ['do', 'then', 'else']) {
      if (s[part] == null) continue;
      s[part] = cmpRead(s, part);
      if (!s[part].length) delete s[part];
      else cmpTidy(s[part]);
    }
    if (s.cond && typeof s.cond === 'object') cmpTidy([s.cond]);
  }
}

// cmpSet sets a field of a step (or the tool), leaving it out when empty, as the JSON would.
function cmpSet(obj, key, value) {
  if (value === '' || value === false || value == null || (Array.isArray(value) && !value.length)) delete obj[key];
  else obj[key] = value;
}

function cmpBlocksEditor(d) {
  if (!Array.isArray(d.draft.steps)) d.draft.steps = [];
  cmpTidy(d.draft.steps);
  const wrap = el('div', 'cmp-blocks');
  wrap.append(cmpPalette(d), cmpCanvas(d));
  return wrap;
}

// cmpMake are the steps the palette's actions add.
const cmpMake = {
  tool: () => ({ action: 'tool', tool: '' }),
  run: () => ({ action: 'run', path: '' }),
  'open-ide': () => ({ action: 'open-ide', ide: cmpState.ides[0]?.id || '', project: '' }),
  message: () => ({ action: 'message', text: '' }),
  orient: () => ({ action: 'orient', windows: [] }),
  home: () => ({ action: 'home' }),
  minimize: () => ({ action: 'minimize' }),
  'ip-check': () => ({ action: 'ip-check', want: [] }),
  if: () => ({ action: 'if' }),
  check: () => ({ action: 'check', every: 5 }),
  delay: () => ({ action: 'delay', seconds: 5 }),
  return: () => ({ action: 'return', fail: true }),
};

// cmpPalOpen are the kinds of action whose suggestions are all shown (else the first few).
const cmpPalOpen = new Set();
const cmpPalFew = 4;

function cmpPalette(d) {
  const pal = el('aside', 'cmp-palette');
  // The actions by group, each followed by the steps of it likely wanted here (the browsers and
  // apps found, the IDEs and the projects they opened lately, …).
  const groups = [...new Set(cmpState.kinds.map(k => k.group))];
  for (const g of groups) {
    pal.append(el('div', 'cmp-pal-head', g));
    for (const k of cmpState.kinds.filter(k => k.group === g)) {
      // Opening a project needs an IDE found here.
      if (!cmpMake[k.id] || (k.id === 'open-ide' && !cmpState.ides.length)) continue;
      pal.append(cmpPaletteItem(d, k.name, k.summary, k.id, cmpMake[k.id]));
      const subs = cmpState.suggestions?.[k.id] || [];
      if (!subs.length) continue;
      const box = el('div', 'cmp-pal-subs');
      const open = cmpPalOpen.has(k.id);
      for (const s of open ? subs : subs.slice(0, cmpPalFew)) {
        const item = cmpPaletteItem(d, s.name, s.summary, k.id, () => JSON.parse(JSON.stringify(s.step)), s.icon);
        item.classList.add('sub');
        box.append(item);
      }
      if (subs.length > cmpPalFew) {
        box.append(button(open ? 'Fewer' : `${subs.length - cmpPalFew} more`, 'cmp-pal-more', () => {
          if (open) cmpPalOpen.delete(k.id); else cmpPalOpen.add(k.id);
          pal.replaceWith(cmpPalette(d));
        }));
      }
      pal.append(box);
    }
  }
  pal.append(el('div', 'cmp-pal-head', 'Tools'));
  const search = el('input', 'cmp-pal-search');
  search.placeholder = 'Find a tool';
  search.type = 'search';
  search.value = cmpPalQuery;
  const list = el('div', 'cmp-pal-tools');
  const fill = () => {
    list.textContent = '';
    const q = search.value.trim().toLowerCase();
    const self = `${cmpState.group} ${d.draft.name}`;
    const lines = cmpState.lines.filter(l => l.line !== self && (!q || l.line.toLowerCase().includes(q) || l.summary.toLowerCase().includes(q)));
    for (const l of lines) list.append(cmpPaletteItem(d, l.line, l.summary, 'tool', () => ({ action: 'tool', tool: l.line })));
    if (!lines.length) list.append(el('div', 'cmp-pal-none', 'No tool matches'));
  };
  search.oninput = () => { cmpPalQuery = search.value; fill(); };
  fill();
  pal.append(search, list);
  pal.append(el('div', 'cmp-pal-tip', 'Drag into the script, or click to add at the end. Drag a block back here to remove it.'));
  // A block dragged back onto the palette is removed.
  pal.addEventListener('dragover', e => {
    if (!cmpDragging?.from) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    pal.classList.add('trash');
  });
  pal.addEventListener('dragleave', e => { if (!pal.contains(e.relatedTarget)) pal.classList.remove('trash'); });
  pal.addEventListener('drop', e => {
    pal.classList.remove('trash');
    if (!cmpDragging?.from) return;
    e.preventDefault();
    cmpTake(d, cmpDragging.from);
    cmpChanged(d);
  });
  return pal;
}

function cmpPaletteItem(d, label, summary, kind, make, icon = '') {
  const item = button('', 'blk-mini', () => {
    d.draft.steps.push(make());
    cmpChanged(d);
    const items = cmpMain.querySelectorAll('.cmp-canvas > .cmp-stack > .cmp-item, .cmp-canvas > .cmp-stack > .cmp-par > .cmp-item');
    items[items.length - 1]?.scrollIntoView({ block: 'nearest' });
  });
  item.dataset.kind = kind;
  item.title = summary;
  item.draggable = true;
  if (icon) {
    const img = el('img', 'blk-img');
    img.src = icon;
    img.alt = '';
    item.append(img);
  } else {
    item.innerHTML = svgIcon(cmpKinds[kind]?.icon || '');
  }
  item.append(el('span', null, label));
  item.addEventListener('dragstart', e => {
    cmpDragging = { make: make() };
    e.dataTransfer.effectAllowed = 'copy';
    e.dataTransfer.setData('text/plain', label);
    document.body.classList.add('cmp-dragging');
  });
  item.addEventListener('dragend', cmpDragEnd);
  return item;
}

function cmpDragEnd() {
  cmpDragging = null;
  document.body.classList.remove('cmp-dragging');
  cmpMain.querySelectorAll('.cmp-insert').forEach(x => x.remove());
  cmpMain.querySelectorAll('.over').forEach(x => x.classList.remove('over'));
  cmpMain.querySelector('.cmp-palette')?.classList.remove('trash');
}

function cmpCanvas(d) {
  const canvas = el('div', 'cmp-canvas');
  const hat = el('div', 'blk hat');
  hat.innerHTML = svgIcon('M5 21V4M5 4h11l-2 4 2 4H5');
  hat.append(el('span', 'blk-word', 'When'), el('span', 'blk-chip hat-name', ''), el('span', 'blk-word', 'runs'));
  canvas.append(hat, cmpStack(d, [], 'Drag an action or a tool here'));
  setTimeout(cmpSyncHat);
  return canvas;
}

function cmpSyncHat() {
  const d = cmpCur();
  const n = cmpMain.querySelector('.hat-name');
  if (d && n) n.textContent = d.draft.name || 'this tool';
}

// cmpStack draws the list of steps at prefix (see cmpListFor), steps marked parallel next to one
// another bracketed together, and takes drops on it.
function cmpStack(d, prefix, empty) {
  const steps = prefix.length ? cmpRead(cmpStepAt(d, prefix.slice(0, -1)), prefix[prefix.length - 1]) : d.draft.steps;
  const stack = el('div', 'cmp-stack');
  for (let i = 0; i < steps.length;) {
    let j = i + 1;
    if (steps[i]?.parallel) while (j < steps.length && steps[j]?.parallel) j++;
    if (j - i > 1) {
      const par = el('div', 'cmp-par');
      const tag = el('div', 'cmp-par-tag', 'together');
      tag.title = 'These steps run at the same time';
      par.append(tag);
      for (let k = i; k < j; k++) par.append(cmpItem(d, [...prefix, k]));
      stack.append(par);
    } else {
      stack.append(cmpItem(d, [...prefix, i]));
    }
    i = j;
  }
  stack.append(el('div', 'cmp-end', steps.length ? 'Drop here to add at the end' : empty));
  cmpListDrop(d, stack, prefix);
  return stack;
}

function cmpItem(d, path) {
  const item = el('div', 'cmp-item');
  item.dataset.index = path[path.length - 1];
  item.append(cmpBlock(d, path));
  return item;
}

// cmpListDrop takes drops on a list of steps: before the step under the pointer's half, or at the
// end. A list inside a block takes them before the list around the block does.
function cmpListDrop(d, stack, prefix) {
  const indexAt = y => {
    const items = [...stack.querySelectorAll(':scope > .cmp-item, :scope > .cmp-par > .cmp-item')];
    for (const it of items) {
      const r = it.getBoundingClientRect();
      if (y < r.top + r.height / 2) return { index: Number(it.dataset.index), before: it };
    }
    return { index: items.length, before: stack.querySelector(':scope > .cmp-end') };
  };
  const ok = () => cmpDragging && !(cmpDragging.from && cmpInside(cmpDragging.from, prefix));
  stack.addEventListener('dragover', e => {
    if (!cmpDragging) return;
    e.stopPropagation();
    cmpMain.querySelectorAll('.over').forEach(x => x.classList.remove('over'));
    if (!ok()) { cmpMain.querySelectorAll('.cmp-insert').forEach(x => x.remove()); return; }
    e.preventDefault();
    e.dataTransfer.dropEffect = cmpDragging.from ? 'move' : 'copy';
    const { before } = indexAt(e.clientY);
    let mark = stack.querySelector(':scope > .cmp-insert, :scope > .cmp-par > .cmp-insert');
    cmpMain.querySelectorAll('.cmp-insert').forEach(x => { if (x !== mark) x.remove(); });
    if (!mark) mark = el('div', 'cmp-insert');
    if (mark.nextSibling !== before) before.parentNode.insertBefore(mark, before);
  });
  stack.addEventListener('dragleave', e => { if (!stack.contains(e.relatedTarget)) stack.querySelectorAll('.cmp-insert').forEach(x => x.remove()); });
  stack.addEventListener('drop', e => {
    if (!cmpDragging) return;
    e.stopPropagation();
    if (!ok()) return;
    e.preventDefault();
    let { index } = indexAt(e.clientY);
    const list = cmpListFor(d, prefix);
    let step;
    if (cmpDragging.from) {
      const w = cmpWhere(d, cmpDragging.from);
      if (w.list === list && w.key < index) index--;
      step = cmpTake(d, cmpDragging.from, false);
    } else {
      step = cmpDragging.make;
    }
    list.splice(index, 0, step);
    cmpDragEnd();
    cmpTidy(d.draft.steps);
    cmpChanged(d);
  });
}

// cmpTake removes the step at path and gives it; tidy: parts left empty go too.
function cmpTake(d, path, tidy = true) {
  const w = cmpWhere(d, path);
  let s;
  if (w.key === 'cond') {
    s = w.list.cond;
    delete w.list.cond;
  } else {
    s = w.list.splice(w.key, 1)[0];
  }
  if (tidy) cmpTidy(d.draft.steps);
  return s;
}

// cmpBlock is a step's block: what it does in words and fields, its marks and buttons, and for an
// if or a check the steps inside it.
function cmpBlock(d, path) {
  const step = cmpStepAt(d, path);
  const isCond = path[path.length - 1] === 'cond';
  const kind = cmpKinds[step?.action] ? step.action : 'unknown';
  const blk = el('div', 'blk');
  blk.dataset.kind = kind;
  blk.dataset.key = cmpKey(path);
  blk.classList.toggle('critical', !!step.critical);

  const head = el('div', 'blk-line');
  const grip = button('', 'blk-grip');
  grip.innerHTML = svgIcon(cmpKinds[kind]?.icon || 'M12 8v5M12 16v.1M12 3l10 18H2z');
  // An IDE's block shows its icon.
  const ideIcon = kind === 'open-ide' && cmpIDE(step.ide)?.icon;
  if (ideIcon) {
    grip.innerHTML = '';
    const img = el('img', 'blk-img');
    img.src = ideIcon;
    img.alt = '';
    grip.append(img);
  }
  grip.title = isCond ? 'Drag to move' : 'Drag to move · ↑ ↓ move it a step';
  grip.setAttribute('aria-label', 'Move step ' + cmpKey(path));
  grip.onkeydown = e => {
    if (isCond || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return;
    e.preventDefault();
    const w = cmpWhere(d, path);
    const to = w.key + (e.key === 'ArrowUp' ? -1 : 1);
    if (to < 0 || to >= w.list.length) return;
    const [s] = w.list.splice(w.key, 1);
    w.list.splice(to, 0, s);
    cmpChanged(d);
    cmpMain.querySelector(`.blk[data-key="${cmpKey([...path.slice(0, -1), to])}"] .blk-grip`)?.focus();
  };
  // Dragged by its icon (the grip), so text in its fields can still be selected; the whole block
  // shows under the pointer.
  grip.draggable = true;
  grip.addEventListener('dragstart', e => {
    e.stopPropagation();
    cmpDragging = { from: path };
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', cmpKey(path));
    const r = blk.getBoundingClientRect();
    e.dataTransfer.setDragImage(blk, Math.min(e.clientX - r.left, 40), Math.min(e.clientY - r.top, 30));
    document.body.classList.add('cmp-dragging');
    requestAnimationFrame(() => blk.classList.add('lifted'));
  });
  grip.addEventListener('dragend', () => { blk.classList.remove('lifted'); cmpDragEnd(); });
  // Its words and fields wrap beside the grip; its marks and buttons stay on the right.
  const fields = el('div', 'blk-fields');
  cmpFields(d, step, fields, kind);
  // Its label and marks show as badges after its fields; the toolbar changes them.
  if (step.label) fields.append(el('span', 'blk-badge label', step.label));
  for (const [key, name] of [['critical', 'Critical'], ['parallel', 'Parallel']]) {
    if (!step[key] || isCond) continue;
    const b = button(name, 'blk-badge ' + key, () => { cmpSet(step, key, false); cmpChanged(d); });
    b.title = `${name}: click to turn it off`;
    fields.append(b);
  }
  head.append(grip, fields);

  // The toolbar, over the block's top right while its line is pointed at or has the focus.
  const tags = el('div', 'blk-tags');
  const label = el('input', 'blk-label');
  label.placeholder = 'label';
  label.value = step.label || '';
  label.title = 'A name for this step in the output';
  label.oninput = () => {
    cmpSet(step, 'label', label.value);
    cmpChanged(d, false);
    let badge = fields.querySelector(':scope > .blk-badge.label');
    if (!label.value) { badge?.remove(); return; }
    if (!badge) { badge = el('span', 'blk-badge label'); fields.querySelector(':scope > .blk-badge')?.before(badge) || fields.append(badge); }
    badge.textContent = label.value;
  };
  tags.append(label);
  if (!isCond) {
    tags.append(cmpToggle(d, step, 'critical', 'Critical', 'If it fails, the steps it is in stop here', 'M13 2L4 14h7l-1 8 9-12h-7z'));
    tags.append(cmpToggle(d, step, 'parallel', 'Parallel', 'Runs together with the parallel steps right before and after it', 'M8 4v16M16 4v16'));
    const dup = button('', 'blk-icon', () => {
      const w = cmpWhere(d, path);
      w.list.splice(w.key + 1, 0, JSON.parse(JSON.stringify(step)));
      cmpChanged(d);
    });
    dup.innerHTML = svgIcon('M9 9h11v11H9zM5 15H4V4h11v1');
    dup.title = 'Duplicate';
    tags.append(dup);
  }
  const del = button('', 'blk-icon', () => { cmpTake(d, path); cmpChanged(d); });
  del.innerHTML = svgIcon('M6 6l12 12M18 6L6 18');
  del.title = 'Remove';
  tags.append(del);
  head.append(tags);
  blk.append(head, el('div', 'blk-issues'));

  if (kind === 'check') {
    blk.append(cmpPart(d, 'Try', cmpStack(d, [...path, 'do'], 'Drop the steps to try here')));
  } else if (kind === 'if') {
    const slot = el('div', 'blk-slot');
    if (step.cond) slot.append(cmpBlock(d, [...path, 'cond']));
    else slot.append(el('div', 'blk-slot-empty', 'Drop the condition here: an action, true when it works'));
    cmpCondDrop(d, slot, path);
    blk.append(cmpPart(d, step.not ? 'When this fails' : 'When this works', slot));
    blk.append(cmpPart(d, 'Then', cmpStack(d, [...path, 'then'], 'Drop the steps to run then here')));
    blk.append(cmpPart(d, 'Else', cmpStack(d, [...path, 'else'], 'Drop the steps to run otherwise here')));
  } else if (kind === 'orient') {
    blk.append(cmpPart(d, 'Windows', cmpOrient(d, step)));
  }
  return blk;
}

// cmpPart is a part of an if or a check: its name and what is in it.
function cmpPart(d, name, body) {
  const part = el('div', 'blk-part');
  part.append(el('div', 'blk-part-name', name), body);
  return part;
}

// cmpCondDrop takes a drop into an if's condition (one already there goes after the if).
function cmpCondDrop(d, slot, ifPath) {
  const ok = () => cmpDragging && !(cmpDragging.from && cmpInside(cmpDragging.from, ifPath)) &&
    !['if', 'return'].includes((cmpDragging.from ? cmpStepAt(d, cmpDragging.from) : cmpDragging.make)?.action);
  slot.addEventListener('dragover', e => {
    if (!cmpDragging) return;
    e.stopPropagation();
    cmpMain.querySelectorAll('.cmp-insert').forEach(x => x.remove());
    if (!ok()) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = cmpDragging.from ? 'move' : 'copy';
    slot.classList.add('over');
  });
  slot.addEventListener('dragleave', e => { if (!slot.contains(e.relatedTarget)) slot.classList.remove('over'); });
  slot.addEventListener('drop', e => {
    if (!cmpDragging) return;
    e.stopPropagation();
    if (!ok()) return;
    e.preventDefault();
    const ifStep = cmpStepAt(d, ifPath);
    const around = cmpWhere(d, ifPath).list;
    const step = cmpDragging.from ? cmpTake(d, cmpDragging.from, false) : cmpDragging.make;
    delete step.critical;
    delete step.parallel;
    if (ifStep.cond && Array.isArray(around)) around.splice(around.indexOf(ifStep) + 1, 0, ifStep.cond);
    ifStep.cond = step;
    cmpDragEnd();
    cmpTidy(d.draft.steps);
    cmpChanged(d);
  });
}

function cmpToggle(d, step, key, label, title, icon) {
  const b = button('', 'blk-toggle ' + key);
  b.innerHTML = svgIcon(icon);
  b.append(el('span', null, label));
  b.title = title;
  b.setAttribute('aria-pressed', String(!!step[key]));
  b.onclick = () => { cmpSet(step, key, !step[key]); cmpChanged(d); };
  return b;
}

// Orienting windows ------------------------------------------------------------------------------

// cmpWins are the screens and windows open now (ComposerWindows), asked for when an orient block
// first shows and again on Detect: null before, {error} when they cannot be listed.
let cmpWins = null;
let cmpWinsLoading = false;

async function cmpLoadWindows() {
  if (cmpWinsLoading) return;
  cmpWinsLoading = true;
  try {
    cmpWins = await api().ComposerWindows();
  } catch (e) {
    cmpWins = { error: String(e), screens: [], windows: [], icons: {} };
  }
  cmpWinsLoading = false;
  const scroll = scroller.scrollTop;
  if (page === 'composer' && cmpCur()?.mode === 'blocks') cmpRender();
  scroller.scrollTop = scroll;
}

// cmpScreenOf is the screen a window is on now (where its middle is), the main one if none.
function cmpScreenOf(w) {
  const x = w.x + w.w / 2, y = w.y + w.h / 2;
  return cmpWins.screens.find(s => x >= s.left && x < s.right && y >= s.top && y < s.bottom) || cmpWins.screens.find(s => s.primary) || cmpWins.screens[0];
}

// cmpRuleNow is a rule that keeps a window where it is now: maximized, minimized, or its place in
// percent of its screen.
function cmpRuleNow(w) {
  const rule = { exe: w.exe };
  const s = cmpScreenOf(w);
  if (s && cmpWins.screens.length > 1) rule.monitor = s.n;
  if (w.state === 1) rule.place = 'maximize';
  else if (w.state === -1) rule.place = 'minimize';
  else {
    const W = s.right - s.left, H = s.bottom - s.top;
    const pct = (v, of) => Math.round(Math.min(100, Math.max(0, v / of * 100)) * 10) / 10;
    rule.place = 'custom';
    rule.x = pct(w.x - s.left, W);
    rule.y = pct(w.y - s.top, H);
    rule.w = Math.min(pct(w.w, W), Math.round((100 - rule.x) * 10) / 10);
    rule.h = Math.min(pct(w.h, H), Math.round((100 - rule.y) * 10) / 10);
  }
  return rule;
}

const cmpRuleColors = ['#3b78e7', '#e0457b', '#2e9447', '#c26a00', '#8257e6', '#1d8db5', '#d4a017', '#5f6b7a'];

// cmpOrient is the list of an orient step's windows, how to add more and a preview of the screens.
function cmpOrient(d, step) {
  if (!cmpWins && !cmpWinsLoading) cmpLoadWindows();
  const box = el('div', 'ori');
  const rules = step.windows || (step.windows = []);
  const icon = exe => {
    const w = cmpWins?.windows.find(w => w.exe.toLowerCase() === exe.toLowerCase());
    return w && cmpWins.icons[w.path];
  };
  const preview = el('div', 'ori-preview');
  const drawPreview = () => cmpOrientPreview(preview, rules);

  rules.forEach((r, i) => {
    const row = el('div', 'ori-row');
    const dot = el('span', 'ori-dot', String(i + 1));
    dot.style.background = cmpRuleColors[i % cmpRuleColors.length];
    row.append(dot);
    const ic = icon(r.exe || '');
    if (ic) { const img = el('img', 'blk-img'); img.src = ic; img.alt = ''; row.append(img); }
    // The program: one of those with a window open now, or typed.
    const exe = el('input', 'ori-field mono');
    exe.placeholder = 'program.exe';
    exe.value = r.exe || '';
    exe.spellcheck = false;
    exe.setAttribute('list', 'ori-exes');
    exe.title = 'The window\'s program';
    exe.oninput = () => { cmpSet(r, 'exe', exe.value.trim()); cmpChanged(d, false); };
    exe.onchange = () => cmpChanged(d);
    const title = el('input', 'ori-field');
    title.placeholder = 'any title';
    title.value = r.title || '';
    title.title = 'A part of the window\'s title (ignoring case), to tell its windows apart';
    title.oninput = () => { cmpSet(r, 'title', title.value); cmpChanged(d, false); };
    row.append(exe, el('span', 'ori-word', 'titled'), title, el('span', 'ori-word', 'on'));
    const mon = el('select', 'ori-select');
    const screens = cmpWins?.screens || [];
    const monOpts = [[0, 'Main screen'], ...screens.map(s => [s.n, `Screen ${s.n} (${s.right - s.left}×${s.bottom - s.top}${s.primary ? ', main' : ''})`])];
    if (r.monitor && !screens.some(s => s.n === r.monitor)) monOpts.push([r.monitor, `Screen ${r.monitor} (not connected)`]);
    for (const [v, t] of monOpts) { const o = el('option', null, t); o.value = String(v); mon.append(o); }
    mon.value = String(r.monitor || 0);
    mon.onchange = () => { cmpSet(r, 'monitor', Number(mon.value)); cmpChanged(d, false); drawPreview(); };
    const place = el('select', 'ori-select');
    for (const p of cmpState.places) { const o = el('option', null, p.name); o.value = p.id; place.append(o); }
    place.value = r.place || 'maximize';
    place.onchange = () => {
      r.place = place.value;
      if (r.place !== 'custom') for (const k of ['x', 'y', 'w', 'h']) delete r[k];
      else Object.assign(r, { x: 0, y: 0, w: 50, h: 100 });
      cmpChanged(d);
    };
    row.append(mon, place);
    if (r.place === 'custom') {
      for (const [k, name] of [['x', 'left'], ['y', 'top'], ['w', 'width'], ['h', 'height']]) {
        const f = el('input', 'ori-field num');
        f.type = 'number';
        f.min = '0';
        f.max = '100';
        f.step = '0.1';
        f.value = r[k] ?? '';
        f.title = `${name}, in percent of the screen`;
        f.placeholder = name;
        f.oninput = () => { if (f.value !== '') r[k] = Number(f.value); cmpChanged(d, false); drawPreview(); };
        row.append(f);
      }
      row.append(el('span', 'ori-word', '%'));
    }
    const all = el('label', 'ori-check');
    const box2 = el('input');
    box2.type = 'checkbox';
    box2.checked = !!r.all;
    box2.onchange = () => { cmpSet(r, 'all', box2.checked); cmpChanged(d, false); };
    all.title = 'Every window that matches, not only the first';
    all.append(box2, el('span', null, 'all'));
    const del = button('', 'ori-del', () => { rules.splice(i, 1); cmpChanged(d); });
    del.innerHTML = svgIcon('M6 6l12 12M18 6L6 18');
    del.title = 'Remove';
    row.append(all, del);
    box.append(row);
  });
  if (!rules.length) box.append(el('div', 'blk-slot-empty', 'Add the windows to put in place'));

  // Adding: one of the windows open now (where it is now), or all of them; Detect lists them again.
  const add = el('div', 'ori-add');
  const pick = el('select', 'ori-select');
  const first = el('option', null, cmpWins ? (cmpWins.error ? 'Windows cannot be listed' : 'Add a window open now…') : 'Looking for windows…');
  first.value = '';
  pick.append(first);
  (cmpWins?.windows || []).forEach((w, i) => {
    const o = el('option', null, `${w.exe} — ${w.title}`);
    o.value = String(i);
    o.title = `${w.state === 1 ? 'Maximized' : w.state === -1 ? 'Minimized' : `${w.w}×${w.h} at ${w.x}, ${w.y}`} · ${w.path}`;
    pick.append(o);
  });
  pick.onchange = () => {
    if (pick.value === '') return;
    rules.push(cmpRuleNow(cmpWins.windows[Number(pick.value)]));
    cmpChanged(d);
  };
  const empty = button('Add by name', 'btn small', () => { rules.push({ exe: '', place: 'maximize' }); cmpChanged(d); });
  empty.title = 'A window of a program not open now';
  const allNow = button('All as they are now', 'btn small', () => {
    // Each program's largest window (not a picture-in-picture or a small dialog of it).
    const seen = new Set(rules.map(r => r.exe.toLowerCase()));
    const area = w => (w.state === 1 ? Infinity : w.w * w.h);
    const best = new Map();
    for (const w of cmpWins?.windows || []) {
      const k = w.exe.toLowerCase();
      if (!seen.has(k) && (!best.has(k) || area(w) > area(best.get(k)))) best.set(k, w);
    }
    for (const w of best.values()) rules.push(cmpRuleNow(w));
    cmpChanged(d);
  });
  allNow.title = 'Each program with a window open now, its largest window where it is now';
  allNow.disabled = !cmpWins?.windows?.length;
  const detect = button('', 'btn small quiet', () => { cmpWins = null; cmpLoadWindows(); });
  detect.innerHTML = svgIcon('M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7') + '<span>Detect</span>';
  detect.classList.add('ori-detect');
  detect.title = 'List the windows open now again';
  add.append(pick, empty, allNow, detect);
  box.append(add);
  if (cmpWins?.error) box.append(el('div', 'ori-error', cmpWins.error));

  // The programs with a window open now, offered as the program is typed.
  const list = el('datalist');
  list.id = 'ori-exes';
  for (const exe of new Set((cmpWins?.windows || []).map(w => w.exe))) { const o = el('option'); o.value = exe; list.append(o); }
  box.append(list);
  box.append(preview);
  drawPreview();
  return box;
}

// cmpOrientPreview draws the screens, as they are arranged, with where each rule puts its window.
function cmpOrientPreview(box, rules) {
  box.textContent = '';
  const screens = cmpWins?.screens || [];
  if (!screens.length) return;
  const minX = Math.min(...screens.map(s => s.left)), minY = Math.min(...screens.map(s => s.top));
  const maxX = Math.max(...screens.map(s => s.right)), maxY = Math.max(...screens.map(s => s.bottom));
  const scale = Math.min(520 / (maxX - minX), 160 / (maxY - minY));
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('width', Math.round((maxX - minX) * scale));
  svg.setAttribute('height', Math.round((maxY - minY) * scale));
  const rect = (x, y, w, h, cls, fill) => {
    const r = document.createElementNS(ns, 'rect');
    Object.entries({ x, y, width: Math.max(w, 1), height: Math.max(h, 1), rx: 3 }).forEach(([k, v]) => r.setAttribute(k, v));
    r.setAttribute('class', cls);
    if (fill) r.style.fill = fill;
    svg.append(r);
    return r;
  };
  const text = (x, y, t, cls) => {
    const e = document.createElementNS(ns, 'text');
    e.setAttribute('x', x);
    e.setAttribute('y', y);
    e.setAttribute('class', cls);
    e.textContent = t;
    svg.append(e);
  };
  for (const s of screens) {
    rect((s.left - minX) * scale, (s.top - minY) * scale, (s.right - s.left) * scale, (s.bottom - s.top) * scale, 'ori-screen');
    text((s.left - minX) * scale + 6, (s.bottom - minY) * scale - 6, `${s.n}${s.primary ? ' · main' : ''}`, 'ori-screen-name');
  }
  const places = Object.fromEntries(cmpState.places.map(p => [p.id, p.rect]));
  rules.forEach((r, i) => {
    const s = screens.find(s => s.n === r.monitor) || screens.find(s => s.primary) || screens[0];
    if (r.place === 'minimize') return;
    const pr = r.place === 'custom' ? [r.x ?? 0, r.y ?? 0, r.w ?? 100, r.h ?? 100] : places[r.place];
    if (!pr) return;
    const W = (s.right - s.left) * scale, H = (s.bottom - s.top) * scale;
    const x = (s.left - minX) * scale + W * pr[0] / 100, y = (s.top - minY) * scale + H * pr[1] / 100;
    const c = cmpRuleColors[i % cmpRuleColors.length];
    rect(x + 1.5, y + 1.5, W * pr[2] / 100 - 3, H * pr[3] / 100 - 3, 'ori-win', c);
    text(x + 7, y + 15, `${i + 1} ${r.exe || ''}`, 'ori-win-name');
  });
  box.append(svg);
}

// cmpFields puts a step's words and fields in line, by its action.
function cmpFields(d, step, line, kind) {
  const word = t => line.append(el('span', 'blk-word', t));
  const text = (key, placeholder, opts = {}) => {
    const f = el('input', 'blk-field' + (opts.mono ? ' mono' : ''));
    f.dataset.field = key;
    f.placeholder = placeholder;
    f.spellcheck = false;
    f.value = opts.get ? opts.get() : (step[key] ?? '');
    f.oninput = () => {
      if (opts.set) opts.set(f.value); else cmpSet(step, key, f.value);
      cmpChanged(d, false);
    };
    line.append(f);
    return f;
  };
  const browse = (key, kind, title, field) => {
    const b = button('', 'blk-browse');
    b.innerHTML = svgIcon(kind === 'folder' ? 'M3 6h6l2 2h10v11H3z' : 'M6 3h8l5 5v13H6zM14 3v5h5');
    b.title = title;
    b.onclick = async () => {
      const p = await api().ComposerBrowse(kind, title, field.value).catch(() => '');
      if (!p) return;
      field.value = p;
      field.dispatchEvent(new Event('input'));
    };
    line.append(b);
  };
  const number = (key, placeholder, title, opts = {}) => {
    const f = el('input', 'blk-field num');
    f.type = 'number';
    f.min = '0';
    if (opts.fraction) f.step = 'any';
    f.dataset.field = key;
    f.placeholder = placeholder;
    f.title = title;
    f.value = step[key] ?? '';
    f.oninput = () => {
      const v = f.value.trim();
      if (v === '') delete step[key];
      else step[key] = Math.max(0, opts.fraction ? Number(v) || 0 : Math.round(Number(v)) || 0);
      cmpChanged(d, false);
    };
    line.append(f);
  };
  const select = (key, options, title) => {
    const s = el('select', 'blk-select');
    s.dataset.field = key;
    s.title = title;
    for (const o of options) {
      const opt = el('option', null, o.label);
      opt.value = o.value;
      if (o.title) opt.title = o.title;
      s.append(opt);
    }
    s.value = step[key] ?? '';
    s.onchange = () => { cmpSet(step, key, s.value); cmpChanged(d, false); };
    line.append(s);
  };
  const list = (key, placeholder) => {
    const f = text(key, placeholder, {
      get: () => (step[key] || []).join(', '),
      set: v => cmpSet(step, key, v.split(',').map(x => x.trim()).filter(Boolean)),
    });
    f.title = 'Country codes (US, DE), or country, region or city names, comma-separated';
  };

  switch (kind) {
    case 'tool': {
      word('Run tool');
      const lines = cmpState.lines.filter(l => l.line !== `${cmpState.group} ${d.draft.name}`);
      const opts = [{ value: '', label: 'Pick a tool…' }, ...lines.map(l => ({ value: l.line, label: l.line, title: l.summary }))];
      if (step.tool && !lines.some(l => l.line === step.tool)) opts.push({ value: step.tool, label: step.tool + ' (not there now)' });
      select('tool', opts, 'The tool to run');
      word('with');
      text('args', 'no args', { mono: true });
      break;
    }
    case 'run': {
      word('Run program');
      const p = text('path', 'program or path', { mono: true });
      browse('path', 'file', 'Choose the program', p);
      word('with');
      text('args', 'no args', { mono: true });
      word('in');
      const dir = text('dir', 'aex’s folder', { mono: true });
      browse('dir', 'folder', 'Choose the folder it runs in', dir);
      const check = (key, label, title) => {
        const l = el('label', 'blk-check');
        const box = el('input');
        box.type = 'checkbox';
        box.checked = !!step[key];
        box.onchange = () => { cmpSet(step, key, box.checked); cmpChanged(d, false); };
        l.title = title;
        l.append(box, el('span', null, label));
        line.append(l);
      };
      check('detach', 'don’t wait', 'Start it and go on without waiting for it to exit (for a program that stays open)');
      check('skipRunning', 'skip if running', 'Don’t start it when it is running already: the step just works');
      break;
    }
    case 'open-ide': {
      word('Open');
      const p = text('project', 'project, solution or folder', { mono: true });
      browse('project', 'file', 'Choose the project or solution', p);
      browse('project', 'folder', 'Choose the folder', p);
      const found = cmpIDE(step.ide);
      // The projects the IDE opened lately: picking one fills the project in.
      if (found?.projects.length) {
        const recent = el('select', 'blk-select blk-recent');
        recent.title = `Projects ${found.name} opened lately`;
        const none = el('option', null, 'Recent');
        none.value = '';
        recent.append(none);
        for (const pr of found.projects) {
          const o = el('option', null, pr.name);
          o.value = pr.path;
          o.title = pr.path + (pr.opened ? ' · ' + new Date(pr.opened).toLocaleDateString() : '');
          recent.append(o);
        }
        recent.onchange = () => {
          if (!recent.value) return;
          p.value = recent.value;
          p.dispatchEvent(new Event('input'));
          recent.value = '';
          recent.dispatchEvent(new Event('change'));
        };
        line.append(recent);
      }
      word('in');
      // Only the IDEs found here; one a step names that is not found stays, marked.
      const opts = cmpState.ides.map(i => ({ value: i.id, label: i.name, title: `${i.provider}${i.version ? ' · ' + i.version : ''} · ${i.path}` }));
      if (!step.ide) opts.unshift({ value: '', label: 'Pick an IDE…' });
      else if (!cmpState.ides.some(i => i.id === step.ide)) opts.push({ value: step.ide, label: (found ? found.name : step.ide) + (found ? '' : ' (not found)') });
      const pick = el('select', 'blk-select');
      pick.dataset.field = 'ide';
      pick.title = 'The IDE';
      for (const o of opts) {
        const opt = el('option', null, o.label);
        opt.value = o.value;
        if (o.title) opt.title = o.title;
        pick.append(opt);
      }
      pick.value = step.ide ?? '';
      // Another IDE has other projects and another icon: the block is drawn again.
      pick.onchange = () => { if (pick.value) { cmpSet(step, 'ide', pick.value); cmpChanged(d); } };
      line.append(pick);
      break;
    }
    case 'if': {
      word('If');
      const not = el('label', 'blk-check');
      const box = el('input');
      box.type = 'checkbox';
      box.checked = !!step.not;
      box.onchange = () => { cmpSet(step, 'not', box.checked); cmpChanged(d); };
      not.title = 'Run then when the condition fails, else when it works';
      not.append(box, el('span', null, 'not'));
      line.append(not);
      break;
    }
    case 'return': {
      word('Return');
      const s = el('select', 'blk-select');
      s.dataset.field = 'fail';
      for (const [v, t] of [['ok', 'worked'], ['fail', 'failed']]) {
        const o = el('option', null, t);
        o.value = v;
        s.append(o);
      }
      s.value = step.fail ? 'fail' : 'ok';
      s.title = 'Ends the steps it is in: one try of the try around it, or else the tool';
      s.onchange = () => { cmpSet(step, 'fail', s.value === 'fail'); cmpChanged(d, false); };
      line.append(s);
      word('saying');
      text('message', 'nothing');
      break;
    }
    case 'message': {
      word('Show message');
      text('title', 'title');
      const t = text('text', 'the message');
      t.classList.add('wide');
      for (const [key, name, title] of [['urgent', 'urgent', 'Red, and chimes until it is closed'], ['wait', 'wait until closed', 'The next step runs once the message is closed']]) {
        const l = el('label', 'blk-check');
        const box = el('input');
        box.type = 'checkbox';
        box.checked = !!step[key];
        box.onchange = () => { cmpSet(step, key, box.checked); cmpChanged(d, false); };
        l.title = title;
        l.append(box, el('span', null, name));
        line.append(l);
      }
      break;
    }
    case 'delay':
      word('Wait');
      number('seconds', '5', 'Seconds to wait', { fraction: true });
      word('seconds');
      break;
    case 'home':
      word('Switch the aex window to Home');
      break;
    case 'minimize':
      word('Minimize the aex window');
      break;
    case 'orient':
      word('Orient windows, waiting up to');
      number('wait', '30', 'Seconds to wait for windows not open yet (0: only those open now)');
      word('s for those not open yet');
      break;
    case 'check':
      word('Try until it works, every');
      number('every', '5', 'Seconds between tries');
      word('s, give up after');
      number('timeout', '300', 'Seconds to give up after (0: never)');
      word('s, at most');
      number('attempts', '∞', 'Tries at most (empty or 0: no limit)');
      word('tries');
      break;
    case 'ip-check':
      word('IP is in');
      list('want', 'anywhere');
      word('and not in');
      list('avoid', 'nowhere');
      break;
    default: {
      word(`Unknown action “${step.action ?? ''}”`);
      const raw = el('code', 'blk-raw', JSON.stringify(step));
      line.append(raw);
    }
  }
}

// Changes, checks and saving ---------------------------------------------------------------------

// cmpChanged follows a change to d's draft: redrawn when the steps changed (rerender), checked soon.
function cmpChanged(d, rerender = true) {
  if (d.mode === 'blocks') d.text = null;
  if (rerender) {
    const scroll = scroller.scrollTop;
    cmpRender();
    scroller.scrollTop = scroll;
  }
  cmpRenderNav();
  cmpScheduleCheck(d);
  cmpShowResult(d);
}

function cmpScheduleCheck(d) {
  clearTimeout(cmpCheckTimer);
  cmpCheckTimer = setTimeout(async () => {
    const key = [...cmpDrafts].find(([, v]) => v === d)?.[0];
    if (key == null) return;
    const text = d.text != null ? d.text : cmpText(d);
    try { JSON.parse(text); } catch (e) {
      d.result = { error: 'JSON: ' + e.message, problems: [] };
      if (d === cmpCur()) cmpShowResult(d);
      return;
    }
    const res = await api().ComposerCheck(key, text).catch(e => ({ error: String(e), problems: [] }));
    d.result = res;
    if (d === cmpCur()) cmpShowResult(d);
  }, 250);
}

// cmpShowResult shows what the last check found: on each block, and above the editor.
function cmpShowResult(d) {
  if (d !== cmpCur()) return;
  const r = d.result || { problems: [] };
  const general = cmpMain.querySelector('.cmp-general');
  const status = cmpMain.querySelector('.cmp-status');
  if (!general || !status) return;
  general.textContent = '';
  cmpMain.querySelectorAll('.blk-issues').forEach(x => (x.textContent = ''));
  cmpMain.querySelectorAll('.blk.bad, .blk.warned').forEach(x => x.classList.remove('bad', 'warned'));
  cmpMain.querySelectorAll('[data-field].bad, .xs-btn.bad').forEach(x => x.classList.remove('bad'));
  // Fields whose message names them already; the others say which field first.
  const plain = ['name', 'steps', 'tool', 'path', 'ide', 'project', 'do', 'want', 'parallel', 'cond', 'then', 'text'];
  const msg = p => (p.field && !plain.includes(p.field) ? p.field + ': ' : '') + p.message;
  if (r.error) general.append(el('div', 'cmp-problem bad', r.error));
  for (const p of r.problems || []) {
    const blk = p.step && cmpMain.querySelector(`.blk[data-key="${p.step}"]`);
    if (!blk) {
      general.append(el('div', 'cmp-problem' + (p.warning ? ' warn' : ' bad'), (p.step ? `Step ${p.step}: ` : '') + msg(p)));
      if (p.field === 'name' && !p.warning) cmpMain.querySelector('.cmp-name')?.classList.add('bad');
      continue;
    }
    blk.classList.add(p.warning ? 'warned' : 'bad');
    const issues = blk.querySelector(':scope > .blk-issues');
    issues.append(el('div', 'blk-issue' + (p.warning ? ' warn' : ''), msg(p)));
    if (!p.warning) {
      const f = blk.querySelector(`:scope > .blk-line [data-field="${p.field}"]`);
      f?.classList.add('bad');
      f?.xsButton?.classList.add('bad');
    }
  }
  if (!r.problems?.some(p => p.field === 'name' && !p.warning)) cmpMain.querySelector('.cmp-name')?.classList.remove('bad');
  const errors = (r.problems || []).filter(p => !p.warning).length + (r.error ? 1 : 0);
  const warnings = (r.problems || []).filter(p => p.warning).length;
  const dirty = cmpDirty(d);
  const parts = [];
  if (errors) parts.push(`${errors} to fix before saving`);
  if (warnings) parts.push(`${warnings} warning${warnings === 1 ? '' : 's'}`);
  if (!parts.length) parts.push(dirty ? 'Not saved' : cmpOpen === '' ? 'Not saved yet' : 'Saved');
  else if (dirty) parts.push('not saved');
  status.textContent = parts.join(' · ');
  status.className = 'cmp-status' + (errors ? ' bad' : warnings ? ' warn' : '');
  const save = cmpMain.querySelector('.cmp-save'), discard = cmpMain.querySelector('.cmp-discard');
  if (save) save.disabled = !dirty || errors > 0;
  if (discard) discard.disabled = !dirty;
}

// cmpSave saves the tool shown; it reports whether it did.
async function cmpSave() {
  const d = cmpCur();
  if (!d) return false;
  const text = d.text != null ? d.text : cmpText(d);
  let res;
  try {
    res = await api().ComposerSave(cmpOpen, text);
  } catch (e) {
    d.result = { error: String(e), problems: [] };
    cmpShowResult(d);
    return false;
  }
  d.result = res;
  if (!res.saved) { cmpShowResult(d); return false; }
  const name = d.draft.name;
  cmpDrafts.delete(cmpOpen);
  cmpOpen = name;
  cmpState = await api().Composer();
  const kept = cmpState.tools.find(t => t.name === name);
  d.saved = kept ? kept.json : text;
  d.text = d.mode === 'json' ? d.saved : null;
  try { d.draft = JSON.parse(d.saved); } catch {}
  cmpDrafts.set(name, d);
  cmpRenderNav();
  cmpRender();
  // The sidebar's tool list has it now.
  if (res.reloaded) await refresh();
  return true;
}

function cmpDiscard() {
  const d = cmpCur();
  if (!d) return;
  if (cmpOpen === '') {
    cmpDrafts.delete('');
    cmpOpen = cmpState.tools[0]?.name ?? null;
    if (cmpOpen != null) cmpEnsure(cmpOpen);
  } else {
    cmpDrafts.delete(cmpOpen);
    cmpEnsure(cmpOpen);
  }
  cmpRenderNav();
  cmpRender();
}

async function cmpDelete() {
  const name = cmpOpen;
  try {
    const reloaded = await api().ComposerDelete(name);
    cmpDrafts.delete(name);
    cmpOpen = null;
    await onComposerPage();
    if (reloaded) await refresh();
  } catch (e) {
    cmpFlash(String(e));
  }
}

// cmpRun saves the tool when it changed, then runs it on Runs.
async function cmpRun() {
  const d = cmpCur();
  if (!d) return;
  if (run) { cmpFlash('A tool is running: wait for it to finish'); return; }
  if ((cmpDirty(d) || cmpOpen === '') && !(await cmpSave())) return;
  runLine(`${cmpState.group} ${cmpCur().draft.name}`);
}

// Ctrl+S saves on the page.
document.addEventListener('keydown', e => {
  if (page !== 'composer' || !(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== 's' || e.target.matches?.('.cmp-json-text')) return;
  e.preventDefault();
  if (cmpCur()) cmpSave();
});
