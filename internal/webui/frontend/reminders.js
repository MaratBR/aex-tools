// Reminders, a section of the Settings page (settings.js): reminders set up to show at a time on
// the days picked, while aex runs, each with a title, a message and links. Backend: reminder.go
// (Reminders, SaveReminders, Remind); they are kept in reminders.json in the data folder, and
// internal/reminder shows them when due.
const remindersPane = $('reminders-pane');

let reminders = [];    // as saved: {id, title, message, time, days, off}
let remindersTZ = '';
let reminderEditing = null;    // the reminder being edited ({} for a new one), null for the list

const dayNames = Object.fromEntries(weekDays);

// daysText is how days read: Every day, Workdays, Weekends, or the days.
function daysText(days) {
  const d = weekDays.map(w => w[0]).filter(x => days.includes(x));
  if (d.length === 7) return 'Every day';
  if (d.join() === 'mon,tue,wed,thu,fri') return 'Workdays';
  if (d.join() === 'sat,sun') return 'Weekends';
  return d.map(x => dayNames[x]).join(', ');
}

function remindersNote(text, isError = false) {
  const n = $('reminders-note');
  if (!n) return;
  n.textContent = text || '';
  n.classList.toggle('error', isError);
}

async function loadReminders(message) {
  try {
    const info = await api().Reminders();
    reminders = info.reminders || [];
    remindersTZ = info.tz || '';
  } catch (e) {
    reminderEditing = null;
    renderReminders();
    return remindersNote(String(e), true);
  }
  renderReminders();
  remindersNote(message);
}

// saveReminders saves list, all of them; the list shown is what was saved.
async function saveReminders(list, message) {
  const saved = await api().SaveReminders(list.map(({ id, title, message, time, days, off, urgent }) => ({ id, title, message, time, days, off, urgent: !!urgent })));
  reminders = saved || [];
  reminderEditing = null;
  renderReminders();
  remindersNote(message);
}

function renderReminders() {
  remindersPane.textContent = '';
  if (reminderEditing) return renderReminderEditor();
  const g = el('section', 'group');
  g.appendChild(el('h3', null, 'Reminders'));
  const card = el('div', 'card');
  if (!reminders.length) card.appendChild(el('div', 'row reminders-empty', 'No reminders yet.'));
  for (const r of reminders) card.appendChild(reminderRow(r));
  card.appendChild(button('Add reminder…', 'row reminders-add', () => { reminderEditing = {}; renderReminders(); }));
  g.appendChild(card);
  g.appendChild(el('p', 'group-foot', `They show on top of every window on every screen, with a chime, while aex runs` +
    `${remindersTZ ? `; times are in ${remindersTZ} (Timezone in General)` : ''}. Start aex when you log in (General) so none is missed; ` +
    'one due while aex was closed shows if it opens within 30 minutes.'));
  remindersPane.appendChild(g);
  remindersPane.appendChild(remindersFoot());
}

function remindersFoot() {
  const foot = el('div', 'reminders-foot');
  const n = el('span', 'settings-note');
  n.id = 'reminders-note';
  foot.appendChild(n);
  return foot;
}

function reminderRow(r) {
  const row = el('div', 'row reminder-row');
  row.classList.toggle('off', !!r.off);
  const body = button('', 'reminder-open', () => { reminderEditing = r; renderReminders(); });
  body.append(el('span', 'reminder-time', r.time));
  const text = el('span', 'reminder-text');
  const head = el('span', 'reminder-title', r.title || r.message.split('\n')[0]);
  if (r.urgent) head.prepend(el('span', 'reminder-urgent', 'Extra urgent'));
  text.append(head,
    el('span', 'reminder-sub', daysText(r.days) + (r.title ? ' · ' + r.message.replace(/\s+/g, ' ') : '')));
  body.append(text);
  body.title = 'Edit';
  const toggle = el('input', 'switch');
  toggle.type = 'checkbox';
  toggle.setAttribute('role', 'switch');
  toggle.setAttribute('aria-label', 'On');
  toggle.checked = !r.off;
  toggle.onchange = async () => {
    toggle.disabled = true;
    try {
      await saveReminders(reminders.map(x => (x.id === r.id ? { ...x, off: !toggle.checked } : x)),
        toggle.checked ? `On: ${r.time}, ${daysText(r.days)}.` : 'Off.');
    } catch (e) {
      toggle.checked = !toggle.checked;
      toggle.disabled = false;
      remindersNote(String(e), true);
    }
  };
  row.append(body, toggle);
  return row;
}

// renderReminderEditor is the form for reminderEditing (or adding) one reminder.
function renderReminderEditor() {
  const r = reminderEditing;
  const isNew = !r.id;
  const g = el('section', 'group');
  g.appendChild(el('h3', null, isNew ? 'New reminder' : 'Edit reminder'));
  const card = el('div', 'card');

  const field = (name, input, hint) => {
    const f = el('div', 'field');
    const head = el('label', 'field-head');
    head.htmlFor = input.id = 'reminder-' + name.toLowerCase();
    head.appendChild(el('span', 'field-name', name));
    const line = el('div', 'field-line');
    line.appendChild(input);
    f.append(head, line);
    if (hint) {
      const h = el('div', 'field-hint');
      h.append(...[hint].flat());
      f.appendChild(h);
    }
    card.appendChild(f);
    return f;
  };

  const time = el('input');
  time.type = 'time';
  time.required = true;
  time.value = r.time || '09:00';
  field('Time', time, remindersTZ ? `In ${remindersTZ}.` : '');

  // Days: a box each, and quick picks.
  const days = el('div', 'field reminder-days');
  days.appendChild(el('span', 'field-head field-name', 'Days'));
  const line = el('div', 'field-line');
  const seg = el('div', 'segmented');
  seg.setAttribute('role', 'group');
  seg.setAttribute('aria-label', 'Days');
  const picked = r.days || ['mon', 'tue', 'wed', 'thu', 'fri'];
  const boxes = weekDays.map(([value, text]) => {
    const label = el('label');
    const input = el('input');
    input.type = 'checkbox';
    input.value = value;
    input.checked = picked.includes(value);
    label.append(input, el('span', null, text));
    seg.appendChild(label);
    return input;
  });
  const pick = list => boxes.forEach(b => (b.checked = list.includes(b.value)));
  line.append(seg,
    button('Workdays', 'btn quiet small', () => pick(['mon', 'tue', 'wed', 'thu', 'fri'])),
    button('Every day', 'btn quiet small', () => pick(weekDays.map(w => w[0]))));
  days.appendChild(line);
  card.appendChild(days);

  const title = el('input');
  title.type = 'text';
  title.maxLength = 100;
  title.placeholder = 'Reminder';
  title.value = r.title || '';
  field('Title', title, 'Optional.');

  const message = el('textarea');
  message.rows = 4;
  message.maxLength = 1000;
  message.placeholder = 'Stand-up in 5 minutes: [open the board](aex+brave://jira.example.com/board)';
  message.value = r.message || '';
  const code = t => el('code', null, t);
  field('Message', message, ['Links: ', code('[label](https://…)'), ' or a bare ', code('https://…'),
    ' open in the default browser; ', code('aex+brave://…'), ' in Brave (also chrome, edge, firefox, vivaldi, yandex, opera, …), or the default one when it is not installed. ',
    'A ', code('!'), ' before the link closes the reminder once it opens: ', code('[Join](!https://…)'), ' or ', code('!https://…'), '.']);

  // Extra urgent: a switch row, with what it does under it.
  const urgentRow = el('label', 'row reminder-urgent-row');
  const urgentText = el('span', 'row-text');
  urgentText.append(el('span', 'field-name', 'Extra urgent'),
    el('span', 'field-hint', 'Chimes twice, then twice again every 30 seconds until you close it or 10 minutes pass. Marked red.'));
  const urgent = el('input', 'switch');
  urgent.type = 'checkbox';
  urgent.setAttribute('role', 'switch');
  urgent.checked = !!r.urgent;
  urgentRow.append(urgentText, urgent);
  card.appendChild(urgentRow);

  const value = () => ({ ...r, time: time.value, days: boxes.filter(b => b.checked).map(b => b.value), title: title.value.trim(), message: message.value.trim(), urgent: urgent.checked });
  const problem = v => !/^\d\d:\d\d$/.test(v.time) ? 'Pick a time.' : !v.days.length ? 'Pick at least one day.' : !v.message ? 'Write a message.' : '';

  g.appendChild(card);
  const actions = el('div', 'reminders-foot');
  const n = el('span', 'settings-note');
  n.id = 'reminders-note';
  const save = button(isNew ? 'Add' : 'Save', 'btn primary', async () => {
    const v = value();
    const p = problem(v);
    if (p) return remindersNote(p, true);
    save.disabled = true;
    try {
      const list = isNew ? [...reminders, v] : reminders.map(x => (x.id === r.id ? v : x));
      await saveReminders(list, `${isNew ? 'Added' : 'Saved'}: ${v.time}, ${daysText(v.days)}.`);
    } catch (e) {
      save.disabled = false;
      remindersNote(String(e), true);
    }
  });
  const preview = button('Show now', 'btn', async () => {
    const v = value();
    if (!v.message) return remindersNote('Write a message.', true);
    try {
      await api().Remind(v.title, v.message, v.urgent);
      remindersNote('Shown: this is how it looks.');
    } catch (e) { remindersNote(String(e), true); }
  });
  const cancel = button('Cancel', 'btn', () => { reminderEditing = null; renderReminders(); });
  if (!isNew) {
    actions.appendChild(button('Delete', 'btn destructive', async () => {
      try {
        await saveReminders(reminders.filter(x => x.id !== r.id), 'Deleted.');
      } catch (e) { remindersNote(String(e), true); }
    }));
  }
  actions.append(n, preview, cancel, save);
  remindersPane.append(g, actions);
  message.addEventListener('keydown', e => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) save.click(); });
  (isNew ? message : time).focus();
}
