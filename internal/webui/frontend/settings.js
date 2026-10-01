// The Settings page: its menu has General, the form of what the configure tool asks for in a
// terminal, Widgets (the same form: the settings of widgets, by widget), Reminders (reminders.js), About (version, build, licenses), and under Plugins each plugin with settings of its
// own, run on the page (a run like on Runs, app.js, but here). Backend: settings.go (Settings,
// SaveSettings, SetShortcut, SetAutostart, WipeList, Wipe, WipeSession), about.go (About), webui.go
// (RunSettings).
const settingsForm = $('settings-form'), settingsBody = $('settings-body');
const saveButton = $('settings-save'), discardButton = $('settings-discard'), settingsNote = $('settings-note');
const pluginPane = $('plugin-settings'), pluginRun = $('plugin-settings-run'), aboutPane = $('about-pane');

const settingLabels = {
  AEXT_EMAIL: 'AEXT email',
  JIRA_EMAIL: 'Jira email',
  JIRA_TOKEN: 'Jira API token',
  HOURS_PER_DAY: 'Hours per day',
  QUOTA_WARN_HOURS: 'Nearly full quota (h)',
  TZ_OFFSET_HOURS: 'Timezone',
};

let fields = {};      // setting name -> {f, input, error, cleared}
let settingsSection = 'general'; // 'widgets', 'reminders', 'about', or the name of the plugin shown
let settingsLoaded = false;
let pluginRunning = ''; // the plugin whose settings run now, on the page

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
  settingsNote.textContent = text || '';
  settingsNote.classList.toggle('error', isError);
}

// openSettings shows the Settings page, on section when given ('general', 'widgets', 'reminders', 'about' or a plugin's name).
function openSettings(section) {
  showPage('settings');
  if (section) showSection(section);
}

// onSettingsPage is showPage coming to Settings: the form is loaded again, unless it has changes.
async function onSettingsPage() {
  renderSettingsNav();
  if (settingsLoaded && Object.keys(changes()).length) return;
  try {
    await loadSettings();
    settingsLoaded = true;
  } catch (e) {
    note(String(e), true);
  }
}

// pluginsWithSettings are the tools with settings of their own (plugins): {name, settings}.
const pluginsWithSettings = () => tools.filter(t => t.settings);

function renderSettingsNav() {
  const nav = $('settings-nav');
  nav.textContent = '';
  const item = (section, name) => {
    const b = button(name, 'settings-link', () => showSection(section, true));
    b.dataset.section = section;
    b.setAttribute('aria-current', String(section === settingsSection));
    nav.appendChild(b);
  };
  item('general', 'General');
  item('widgets', 'Widgets');
  item('reminders', 'Reminders');
  item('about', 'About');
  nav.appendChild(el('div', 'settings-nav-head', 'Plugins'));
  const plugins = pluginsWithSettings();
  for (const t of plugins) item(t.name, t.name);
  if (!plugins.length) nav.appendChild(el('div', 'settings-nav-empty', 'No plugin has settings. One not approved yet shows here once it is.'));
  if (!builtinSections.includes(settingsSection) && !plugins.some(t => t.name === settingsSection) && pluginRunning !== settingsSection) showSection('general');
}

// builtinSections are the Settings sections that are not a plugin's.
const builtinSections = ['general', 'widgets', 'reminders', 'about'];

// showSection shows General, Widgets, Reminders, About or a plugin's settings; picked (from the menu) starts the plugin's.
function showSection(section, picked = false) {
  if (section !== settingsSection) leavePluginSettings();
  settingsSection = section;
  document.querySelectorAll('.settings-link').forEach(b => b.setAttribute('aria-current', String(b.dataset.section === section)));
  settingsForm.hidden = section !== 'general' && section !== 'widgets';
  showFormGroups();
  aboutPane.hidden = section !== 'about';
  remindersPane.hidden = section !== 'reminders';
  pluginPane.hidden = builtinSections.includes(section);
  if (section === 'about') loadAbout();
  if (section === 'reminders') { reminderEditing = null; loadReminders(); }
  if (builtinSections.includes(section)) return;
  const t = tools.find(t => t.name === section);
  $('plugin-settings-name').textContent = section;
  $('plugin-settings-summary').textContent = t?.settings || '';
  if (pluginRunning === section) return syncPluginPane();
  pluginRun.textContent = '';
  syncPluginPane();
  if (picked && !run) startPluginSettings();
}

// syncPluginPane shows whether the plugin shown can be run now.
function syncPluginPane() {
  const busy = !!run;
  $('plugin-settings-open').hidden = pluginRunning === settingsSection;
  $('plugin-settings-open').disabled = busy;
  $('plugin-settings-open').textContent = pluginRun.children.length ? 'Change again' : 'Change settings';
  $('plugin-settings-note').textContent = busy && pluginRunning !== settingsSection ? 'A tool is running: wait for it to finish.' : '';
}

async function startPluginSettings() {
  if (run || builtinSections.includes(settingsSection)) return;
  const name = settingsSection;
  pluginRun.textContent = '';
  pluginRunning = name;
  startRun([name, 'settings'], [], pluginRun);
  syncPluginPane();
  try {
    await api().RunSettings(name);
  } catch (e) {
    finished({ status: String(e), ok: false });
  }
}

// leavePluginSettings is leaving the plugin's settings shown while they run: the question they wait
// on is cancelled, or the run would go on waiting out of sight and keep every other tool from
// running.
function leavePluginSettings() {
  if (pluginRunning && pluginRunning === settingsSection && run?.page === 'settings' && cancelQuestion) cancelQuestion();
}

// pluginSettingsFinished is the plugin's settings run ending (app.js: finished).
function pluginSettingsFinished() {
  pluginRunning = '';
  syncPluginPane();
}

// runChanged is a run starting or ending anywhere: the plugin pane's button follows.
function runChanged() {
  if (!pluginPane.hidden) syncPluginPane();
}

// group adds a titled group of rows to the form and returns where its rows go.
// group adds a group of the form, shown on General, or on Widgets when widgets is set.
function group(title, foot, widgets = false) {
  const g = el('section', 'group');
  g.dataset.section = widgets ? 'widgets' : 'general';
  if (title) g.appendChild(el('h3', null, title));
  const card = el('div', 'card');
  g.appendChild(card);
  if (foot) g.appendChild(el('p', 'group-foot', foot));
  settingsBody.appendChild(g);
  return card;
}

async function loadSettings(message) {
  const form = await api().Settings();
  fields = {};
  settingsBody.textContent = '';
  const groups = [
    ['Accounts', f => !f.default],
    ['Work', f => !!f.default, 'Leave a field empty to go back to its default.'],
  ];
  for (const [title, pick, foot] of groups) {
    const list = form.fields.filter(f => !f.widget && pick(f));
    if (list.length) group(title, foot).append(...list.map(fieldRow));
  }
  const widgets = [...new Set(form.fields.filter(f => f.widget).map(f => f.widget))];
  for (const w of widgets) group(w, 'Leave a field empty to go back to its default.', true).append(...form.fields.filter(f => f.widget === w).map(fieldRow));
  group('Appearance').appendChild(themeRow());
  if (form.shortcut) group('App launcher').appendChild(shortcutRow(form.shortcut));
  if (form.autostart) group('Start with the system', 'On a day not picked, aex started when you log in closes at once.')
    .append(...autostartRows(form.autostart, saveAutostart));
  group('Reset', `Settings are saved in ${form.file}. .env and environment variables are never touched and still apply.`)
    .append(wipeRow(false), wipeRow(true));
  showFormGroups();
  updateDirty();
  note(message);
}

// showFormGroups shows the form's groups of the section shown: General's or Widgets'.
function showFormGroups() {
  settingsBody.querySelectorAll('.group').forEach(g => { g.hidden = g.dataset.section !== settingsSection; });
}

// tzOffsets are the UTC offsets in use somewhere, in hours.
const tzOffsets = [-12, -11, -10, -9.5, -9, -8, -7, -6, -5, -4, -3.5, -3, -2, -1, 0, 1, 2, 3, 3.5, 4, 4.5, 5, 5.5, 5.75,
  6, 6.5, 7, 8, 8.75, 9, 9.5, 10, 10.5, 11, 12, 12.75, 13, 13.75, 14];

// tzZones are well-known timezones, most known first, to name each offset by the places on it now.
const tzZones = ['Pacific/Kwajalein', 'Pacific/Pago_Pago', 'Pacific/Honolulu', 'Pacific/Marquesas',
  'America/Anchorage', 'America/Los_Angeles', 'America/Vancouver', 'America/Tijuana', 'America/Denver',
  'America/Phoenix', 'America/Chicago', 'America/Mexico_City', 'America/Guatemala', 'America/New_York',
  'America/Toronto', 'America/Bogota', 'America/Lima', 'America/Halifax', 'America/Caracas',
  'America/Santiago', 'America/St_Johns', 'America/Sao_Paulo', 'America/Argentina/Buenos_Aires',
  'America/Montevideo', 'America/Noronha', 'Atlantic/South_Georgia', 'Atlantic/Azores', 'Atlantic/Cape_Verde',
  'Europe/London', 'Europe/Lisbon', 'Africa/Casablanca', 'Africa/Accra', 'Atlantic/Reykjavik', 'Europe/Berlin', 'Europe/Paris',
  'Europe/Madrid', 'Europe/Rome', 'Europe/Warsaw', 'Africa/Lagos', 'Europe/Kyiv', 'Europe/Athens',
  'Europe/Bucharest', 'Africa/Cairo', 'Africa/Johannesburg', 'Asia/Jerusalem', 'Europe/Moscow',
  'Europe/Istanbul', 'Asia/Riyadh', 'Africa/Nairobi', 'Asia/Baghdad', 'Asia/Tehran', 'Asia/Dubai',
  'Europe/Samara', 'Asia/Baku', 'Asia/Tbilisi', 'Asia/Yerevan', 'Asia/Kabul', 'Asia/Yekaterinburg',
  'Asia/Tashkent', 'Asia/Karachi', 'Asia/Kolkata', 'Asia/Colombo', 'Asia/Kathmandu', 'Asia/Almaty',
  'Asia/Omsk', 'Asia/Dhaka', 'Asia/Yangon', 'Asia/Bangkok', 'Asia/Jakarta', 'Asia/Novosibirsk',
  'Asia/Krasnoyarsk', 'Asia/Ho_Chi_Minh', 'Asia/Shanghai', 'Asia/Singapore', 'Asia/Hong_Kong',
  'Australia/Perth', 'Asia/Irkutsk', 'Australia/Eucla', 'Asia/Tokyo', 'Asia/Seoul', 'Asia/Yakutsk',
  'Australia/Adelaide', 'Australia/Darwin', 'Australia/Sydney', 'Australia/Melbourne', 'Australia/Brisbane',
  'Asia/Vladivostok', 'Australia/Lord_Howe', 'Asia/Magadan', 'Pacific/Noumea', 'Pacific/Auckland',
  'Pacific/Fiji', 'Asia/Kamchatka', 'Pacific/Chatham', 'Pacific/Tongatapu', 'Pacific/Apia', 'Pacific/Kiritimati'];

// zoneOffset is a timezone's UTC offset now, in hours; NaN when the web view does not know it.
function zoneOffset(zone) {
  try {
    const name = new Intl.DateTimeFormat('en-US', { timeZone: zone, timeZoneName: 'longOffset' })
      .formatToParts(new Date()).find(p => p.type === 'timeZoneName')?.value || '';
    const m = /^GMT(?:([+-])(\d{1,2})(?::(\d{2}))?)?$/.exec(name);
    if (!m) return NaN;
    const hours = Number(m[2] || 0) + Number(m[3] || 0) / 60;
    return m[1] === '-' ? -hours : hours;
  } catch { return NaN; }
}

// zonePlace is a timezone's place name: America/New_York is New York.
const zonePlace = zone => zone.split('/').pop().replace(/_/g, ' ');

// placesByOffset lists, for each offset, the places of tzZones on it now (daylight saving included).
function placesByOffset() {
  const places = new Map();
  for (const zone of tzZones) {
    const h = zoneOffset(zone);
    if (Number.isNaN(h)) continue;
    if (!places.has(h)) places.set(h, []);
    places.get(h).push(zonePlace(zone));
  }
  return places;
}

function offsetLabel(hours) {
  const abs = Math.abs(hours), whole = Math.trunc(abs), minutes = Math.round((abs - whole) * 60);
  return 'UTC' + (hours < 0 ? '-' : '+') + whole + (minutes ? ':' + String(minutes).padStart(2, '0') : '');
}

// zoneLabel is e.g. "Berlin, Paris, Madrid, Rome (UTC+2)": up to 4 places on the offset now, then the offset.
function zoneLabel(places, hours) {
  const names = places.get(hours) || [];
  return names.length ? `${names.slice(0, 4).join(', ')} (${offsetLabel(hours)})` : offsetLabel(hours);
}

// tzSelect picks TZ_OFFSET_HOURS: auto (the device's timezone) or a UTC offset, each named by the
// places on it now. A saved value is matched to its option by number (07 is 7), so it does not show as changed.
function tzSelect(f) {
  const select = el('select');
  const option = (value, text) => {
    const o = el('option', null, text);
    o.value = value;
    select.appendChild(o);
  };
  const places = placesByOffset();
  let device = '';
  try { device = Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch { /* unknown */ }
  const deviceName = device && device !== 'UTC' ? zonePlace(device) + ' ' : '';
  option('auto', `Device timezone: ${deviceName}(${f.auto || 'auto'})`);
  const offsets = [...new Set([...tzOffsets, ...places.keys()])].sort((a, b) => a - b);
  for (const h of offsets) option(String(h), zoneLabel(places, h));
  const saved = f.value || f.default;
  const match = [...select.options].find(o => o.value.toLowerCase() === String(saved).toLowerCase() ||
    (o.value !== 'auto' && Number(o.value) === Number(saved)));
  if (!match) option(saved, saved);
  f.value = match ? match.value : saved;
  select.value = f.value;
  return select;
}

function fieldRow(f) {
  const row = el('div', 'field');
  const id = 'setting-' + f.name;
  const head = el('label', 'field-head');
  head.htmlFor = id;
  head.append(el('span', 'field-name', settingLabels[f.name] || f.name), el('span', 'field-env', f.name));
  const line = el('div', 'field-line');
  const isTZ = f.name === 'TZ_OFFSET_HOURS';
  const input = isTZ ? tzSelect(f) : el('input');
  input.id = id;
  const secretPlaceholder = () => (f.masked ? `Saved (${f.masked}), type to replace` : 'Not set');
  if (isTZ) f.hint = 'Dates are computed in it: ranges, "today", worklog days, file timestamps.';
  else {
    input.type = f.secret ? 'password' : 'text';
    input.spellcheck = false;
    input.autocomplete = 'off';
    input.value = f.value || '';
    input.placeholder = f.secret ? secretPlaceholder() : f.default ? 'Default: ' + f.default : 'Not set';
  }
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
  if (f.warning) row.appendChild(el('div', 'field-note', f.warning));
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

// autostartRows are a switch for starting aex when the user logs in and the days to (Monday first),
// set to info ({enabled, days}). onChange(value) runs on each change with {enabled, days}; a promise
// it returns keeps the rows disabled until it settles, and puts them back as they were when it
// rejects. The last day picked cannot be unpicked.
const weekDays = [['mon', 'Mon'], ['tue', 'Tue'], ['wed', 'Wed'], ['thu', 'Thu'], ['fri', 'Fri'], ['sat', 'Sat'], ['sun', 'Sun']];
function autostartRows(info, onChange) {
  const row = el('label', 'row');
  row.appendChild(el('span', 'row-text', 'Start aex when you log in'));
  const toggle = el('input', 'switch');
  toggle.type = 'checkbox';
  toggle.setAttribute('role', 'switch');
  toggle.checked = info.enabled;
  row.appendChild(toggle);
  const daysRow = el('div', 'row autostart-days');
  daysRow.appendChild(el('span', 'row-text', 'On'));
  const seg = el('div', 'segmented');
  seg.setAttribute('role', 'group');
  seg.setAttribute('aria-label', 'Days to start on');
  const boxes = weekDays.map(([value, text]) => {
    const label = el('label');
    const input = el('input');
    input.type = 'checkbox';
    input.value = value;
    input.checked = info.days.includes(value);
    label.append(input, el('span', null, text));
    seg.appendChild(label);
    return input;
  });
  daysRow.appendChild(seg);
  const value = () => ({ enabled: toggle.checked, days: boxes.filter(b => b.checked).map(b => b.value) });
  const sync = () => { daysRow.classList.toggle('off', !toggle.checked); };
  let last = value();
  const changed = async input => {
    if (!boxes.some(b => b.checked)) { input.checked = true; return; }
    sync();
    const inputs = [toggle, ...boxes];
    inputs.forEach(i => (i.disabled = true));
    try {
      await onChange(value());
      last = value();
    } catch {
      toggle.checked = last.enabled;
      boxes.forEach(b => (b.checked = last.days.includes(b.value)));
      sync();
    }
    inputs.forEach(i => (i.disabled = false));
  };
  [toggle, ...boxes].forEach(i => (i.onchange = () => changed(i)));
  sync();
  return [row, daysRow];
}

async function saveAutostart({ enabled, days }) {
  try {
    await api().SetAutostart(enabled, days);
    note(enabled ? 'aex starts when you log in on ' + days.map(d => weekDays.find(w => w[0] === d)[1]).join(', ') + '.' : 'aex no longer starts when you log in.');
  } catch (e) {
    note(String(e), true);
    throw e;
  }
}

// themeRow picks the window's theme, which applies (and is kept) at once, not on Save.
function themeRow() {
  const row = el('div', 'row');
  row.appendChild(el('span', 'row-text', 'Theme'));
  const seg = el('div', 'segmented');
  seg.setAttribute('role', 'radiogroup');
  seg.setAttribute('aria-label', 'Theme');
  for (const [value, text] of [['system', 'System'], ['light', 'Light'], ['dark', 'Dark']]) {
    const label = el('label');
    const input = el('input');
    input.type = 'radio';
    input.name = 'theme';
    input.value = value;
    input.checked = theme.get() === value;
    input.onchange = () => theme.set(value);
    label.append(input, el('span', null, text));
    seg.appendChild(label);
  }
  row.appendChild(seg);
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

function updateDirty() {
  const dirty = !!Object.keys(changes()).length;
  saveButton.disabled = discardButton.disabled = !dirty;
  settingsForm.classList.toggle('dirty', dirty);
}

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
  settingsBody.prepend(banner);
}

settingsForm.onsubmit = async e => {
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

discardButton.onclick = () => loadSettings().catch(e => note(String(e), true));
$('plugin-settings-open').onclick = startPluginSettings;
