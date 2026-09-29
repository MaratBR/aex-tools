// The settings form: what the configure tool asks for in a terminal, as a sheet over the window.
// Backend: settings.go (Settings, SaveSettings, SetShortcut, WipeList, Wipe, WipeSession).
const sheetBackdrop = $('sheet-backdrop'), sheet = $('sheet'), sheetBody = $('sheet-body');
const saveButton = $('sheet-save'), sheetNote = $('sheet-note');

const settingLabels = {
  AEXT_EMAIL: 'AEXT email',
  JIRA_EMAIL: 'Jira email',
  JIRA_TOKEN: 'Jira API token',
  HOURS_PER_DAY: 'Hours per day',
  TZ_OFFSET_HOURS: 'Timezone',
};

let fields = {};      // setting name -> {f, input, error, cleared}
let lastFocus = null;

// withLinks is text with its URLs opened in the browser (not in the window).
function withLinks(text) {
  const frag = document.createDocumentFragment();
  text.split(' ').forEach((word, i) => {
    if (i) frag.append(' ');
    if (/^https?:\/\//.test(word)) {
      const a = el('a', null, word.replace(/^https?:\/\//, ''));
      a.href = word;
      a.onclick = e => { e.preventDefault(); window.runtime.BrowserOpenURL(word); };
      frag.append(a);
    } else frag.append(word);
  });
  return frag;
}

function note(text, isError = false) {
  sheetNote.textContent = text || '';
  sheetNote.classList.toggle('error', isError);
}

async function openSettings() {
  if (!sheetBackdrop.hidden) return;
  lastFocus = document.activeElement;
  try {
    await loadSettings();
  } catch (e) {
    showCommandError(String(e));
    return;
  }
  sheetBackdrop.hidden = false;
  requestAnimationFrame(() => sheetBackdrop.classList.add('shown'));
  sheetBody.scrollTop = 0;
  sheet.querySelector('.field input')?.focus();
}

function closeSettings() {
  sheetBackdrop.classList.remove('shown');
  sheetBackdrop.hidden = true;
  lastFocus?.focus?.();
}

// group adds a titled group of rows to the form and returns where its rows go.
function group(title, foot) {
  const g = el('section', 'group');
  if (title) g.appendChild(el('h3', null, title));
  const card = el('div', 'card');
  g.appendChild(card);
  if (foot) g.appendChild(el('p', 'group-foot', foot));
  sheetBody.appendChild(g);
  return card;
}

async function loadSettings(message) {
  const form = await api().Settings();
  fields = {};
  sheetBody.textContent = '';
  const groups = [
    ['Accounts', f => !f.default],
    ['Work', f => !!f.default, 'Leave a field empty to go back to its default.'],
  ];
  for (const [title, pick, foot] of groups) {
    const list = form.fields.filter(pick);
    if (list.length) group(title, foot).append(...list.map(fieldRow));
  }
  if (form.shortcut) group('App launcher').appendChild(shortcutRow(form.shortcut));
  group('Reset', `Settings are saved in ${form.file}. .env and environment variables are never touched and still apply.`)
    .append(wipeRow(false), wipeRow(true));
  updateDirty();
  note(message);
}

function fieldRow(f) {
  const row = el('div', 'field');
  const id = 'setting-' + f.name;
  const head = el('label', 'field-head');
  head.htmlFor = id;
  head.append(el('span', 'field-name', settingLabels[f.name] || f.name), el('span', 'field-env', f.name));
  const line = el('div', 'field-line');
  const input = el('input');
  input.id = id;
  input.type = f.secret ? 'password' : 'text';
  input.spellcheck = false;
  input.autocomplete = 'off';
  input.value = f.value || '';
  const secretPlaceholder = () => (f.masked ? `Saved (${f.masked}), type to replace` : 'Not set');
  input.placeholder = f.secret ? secretPlaceholder() : f.default ? 'Default: ' + f.default : 'Not set';
  line.appendChild(input);
  const x = { f, input, cleared: false };
  if (f.secret && f.masked) {
    const remove = button('Remove', 'btn small', () => {
      x.cleared = !x.cleared;
      remove.textContent = x.cleared ? 'Keep' : 'Remove';
      input.value = '';
      input.disabled = x.cleared;
      input.placeholder = x.cleared ? 'Removed when you save' : secretPlaceholder();
      updateDirty();
    });
    line.appendChild(remove);
  }
  row.append(head, line);
  const hint = el('div', 'field-hint');
  hint.appendChild(withLinks(f.hint));
  row.appendChild(hint);
  if (f.source) row.appendChild(el('div', 'field-note', `Now comes from ${f.source}. Saving it here overrides that.`));
  else if (f.overridden?.length) row.appendChild(el('div', 'field-note', `Also set in ${f.overridden.join(', ')}, which this overrides.`));
  x.error = el('div', 'field-error');
  x.error.hidden = true;
  row.appendChild(x.error);
  input.addEventListener('input', () => { x.error.hidden = true; row.classList.remove('invalid'); updateDirty(); });
  fields[f.name] = x;
  return row;
}

function shortcutRow(s) {
  const row = el('label', 'row');
  row.appendChild(el('span', 'row-text', 'Show aex in the ' + s.where));
  const toggle = el('input', 'switch');
  toggle.type = 'checkbox';
  toggle.setAttribute('role', 'switch');
  toggle.checked = s.exists;
  toggle.onchange = async () => {
    toggle.disabled = true;
    try {
      await api().SetShortcut(toggle.checked);
      note(toggle.checked ? 'Added to the ' + s.where + '.' : 'Removed from the ' + s.where + '.');
    } catch (e) {
      toggle.checked = !toggle.checked;
      note(String(e), true);
    }
    toggle.disabled = false;
  };
  row.appendChild(toggle);
  return row;
}

// wipeRow deletes all settings, or (all) everything in the data folder, once CONFIRM is typed.
function wipeRow(all) {
  const box = el('div', 'wipe');
  const panel = el('div', 'wipe-panel');
  const toggle = button(all ? 'Wipe everything…' : 'Wipe all settings…', 'row danger', async () => {
    if (box.classList.contains('open')) {
      box.classList.remove('open');
      panel.textContent = '';
      return;
    }
    let what;
    try { what = await api().WipeList(all); } catch (e) { note(String(e), true); return; }
    panel.textContent = '';
    panel.appendChild(el('div', 'wipe-title', 'This permanently deletes:'));
    const ul = el('ul');
    what.forEach(w => ul.appendChild(el('li', null, w)));
    panel.appendChild(ul);
    if (all) panel.appendChild(el('div', 'field-hint', 'Plugins and their approvals are kept.'));
    const line = el('div', 'field-line');
    const input = el('input');
    input.placeholder = 'Type CONFIRM';
    input.spellcheck = false;
    const go = button(all ? 'Wipe everything' : 'Wipe settings', 'btn destructive', async () => {
      go.disabled = true;
      try {
        await api().Wipe(all);
        await loadSettings(all ? 'Wiped everything.' : 'Wiped all settings.');
        refresh();
      } catch (e) {
        note(String(e), true);
        go.disabled = false;
      }
    });
    go.disabled = true;
    input.addEventListener('input', () => { go.disabled = input.value.trim() !== 'CONFIRM'; });
    input.addEventListener('keydown', e => {
      if (e.key !== 'Enter') return;
      e.preventDefault();
      if (!go.disabled) go.click();
    });
    line.append(input, go);
    panel.appendChild(line);
    box.classList.add('open');
    input.focus();
  });
  box.append(toggle, panel);
  return box;
}

// changes are the edited settings: an empty value removes one from app settings.
function changes() {
  const out = {};
  for (const [name, x] of Object.entries(fields)) {
    const v = x.input.value.trim();
    if (x.f.secret) {
      if (v) out[name] = v;
      else if (x.cleared) out[name] = '';
    } else if (v !== (x.f.value || '')) out[name] = v;
  }
  return out;
}

function updateDirty() { saveButton.disabled = !Object.keys(changes()).length; }

function showOldSession() {
  const banner = el('div', 'banner');
  banner.appendChild(el('div', null, 'AEXT email changed. The saved AEXT session belongs to the old email.'));
  const actions = el('div', 'banner-actions');
  actions.append(
    button('Wipe old session', 'btn primary small', async () => {
      try {
        await api().WipeSession();
        banner.remove();
        note('Session wiped: the next AEXT tool asks for a login code.');
        refresh();
      } catch (e) { note(String(e), true); }
    }),
    button('Keep', 'btn small', () => banner.remove()));
  banner.appendChild(actions);
  sheetBody.prepend(banner);
}

sheet.onsubmit = async e => {
  e.preventDefault();
  const c = changes();
  if (!Object.keys(c).length) return;
  saveButton.disabled = true;
  try {
    const res = await api().SaveSettings(c);
    if (res.errors) {
      let first = null;
      for (const [name, problem] of Object.entries(res.errors)) {
        const x = fields[name];
        if (!x) continue;
        x.error.textContent = problem.charAt(0).toUpperCase() + problem.slice(1) + '.';
        x.error.hidden = false;
        x.input.closest('.field').classList.add('invalid');
        first = first || x.input;
      }
      first?.focus();
      note('');
      updateDirty();
      return;
    }
    await loadSettings((res.saved || []).join(' · '));
    if (res.oldSession) showOldSession();
    refresh();
  } catch (err) {
    note(String(err), true);
    updateDirty();
  }
};

$('open-settings').onclick = openSettings;
$('sheet-close').onclick = closeSettings;
$('sheet-cancel').onclick = closeSettings;
sheetBackdrop.addEventListener('mousedown', e => { if (e.target === sheetBackdrop) closeSettings(); });
// While the sheet is open, keys are its own: a question's shortcuts (y, n, 1-9) must not fire.
window.addEventListener('keydown', e => {
  if (sheetBackdrop.hidden) return;
  if (e.key === 'Escape') { e.preventDefault(); closeSettings(); }
  e.stopPropagation();
}, true);
