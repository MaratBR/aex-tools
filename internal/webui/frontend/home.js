// Pages and the home page. The window has two pages: Runs (the feed, app.js) and Home, a grid of
// widgets. It opens on Home when that has widgets, else on Runs.
//
// Each widget is a page of its own in a sandboxed frame (scripts only: no access to the window, no
// network). Its page comes from the backend (built-in: widgets/<id>.html; a plugin's: from the
// plugin), with the window's colors, widget.css and widgets/sdk.js put at the top of its <head>. It
// reaches the window only through messages, answered here: its data calls and running a tool.
// Backend: home.go (Widgets, Home, SaveHome, WidgetPage, WidgetCall).
const grid = $('grid'), picker = $('widget-picker');

let page = 'runs';
let pageChosen = false; // once the user or a question picked a page, loading Home keeps it
const scrollTops = { home: 0, runs: 0 };
let catalog = { widgets: [], inactive: [] }; // WidgetList
let layout = [];        // HomeWidget: {id, widget, w, h, settings}
let autoRefresh = true; // !HomeLayout.autoRefreshOff
let editing = false;
let cols = 4;

// showPage shows Home or Runs. auto: chosen by the window at start, not by the user.
function showPage(name, auto = false) {
  if (!auto) pageChosen = true;
  if (name === page) return;
  scrollTops[page] = scroller.scrollTop;
  page = name;
  $('home').hidden = name !== 'home';
  $('page').hidden = name !== 'runs';
  document.body.dataset.page = name;
  document.querySelectorAll('.page-link').forEach(b => b.setAttribute('aria-current', String(b.dataset.page === name)));
  if (name === 'runs') document.body.classList.remove('runs-activity');
  if (name !== 'home') closePicker();
  scroller.scrollTop = scrollTops[name];
  if (name === 'home') fitGrid();
}

// runsActivity marks Runs in the sidebar when a run prints while Home is shown.
function runsActivity() {
  if (page !== 'runs') document.body.classList.add('runs-activity');
}

document.querySelectorAll('.page-link').forEach(b => (b.onclick = () => showPage(b.dataset.page)));

// Layout ----------------------------------------------------------------------------------------

// pluginOf is the plugin a widget id comes from ("<plugin>/<id>"), '' for a built-in one.
const pluginOf = id => (id.includes('/') ? id.slice(0, id.indexOf('/')) : '');
const info = id => catalog.widgets.find(w => w.id === id);
const widgetName = id => info(id)?.name || id.slice(id.indexOf('/') + 1);
const newID = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
const cellOf = w => grid.querySelector(`.cell[data-id="${w.id}"]`);

const homeLayout = () => ({ widgets: layout, autoRefreshOff: !autoRefresh });

async function save() {
  try {
    await api().SaveHome(homeLayout());
  } catch (e) {
    showCommandError('Could not save the home page: ' + e);
  }
}

function renderHome() {
  $('home-empty').hidden = layout.length > 0;
  $('home-edit').hidden = !layout.length;
  syncAutoSwitch();
  if (!layout.length && editing) setEditing(false);
  const cells = new Map([...grid.children].map(c => [c.dataset.id, c]));
  for (const w of layout) {
    if (!cells.has(w.id)) grid.appendChild(makeCell(w));
    cells.delete(w.id);
  }
  cells.forEach(c => c.remove());
  fitGrid();
}

// fitGrid sets the columns for the grid's width, and each widget's place (CSS order: moving a
// frame in the page would reload it) and span in them.
function fitGrid() {
  layout.forEach((w, i) => {
    const cell = cellOf(w);
    if (!cell) return;
    cell.style.order = i;
    cell.querySelector('.cell-name').textContent = widgetName(w.widget);
    cell.querySelector('iframe').title = widgetName(w.widget);
  });
  if (page !== 'home') return;
  const width = grid.clientWidth;
  if (!width) return;
  cols = width >= 900 ? 4 : width >= 440 ? 2 : 1;
  grid.style.setProperty('--cols', cols);
  for (const w of layout) {
    const cell = cellOf(w);
    if (!cell) continue;
    cell.style.gridColumn = `span ${Math.min(w.w, cols)}`;
    cell.style.gridRow = `span ${w.h}`;
    cell.querySelector('.size').textContent = `${w.w} × ${w.h}`;
  }
}
new ResizeObserver(() => fitGrid()).observe(grid);

function makeCell(w) {
  const cell = el('div', 'cell');
  cell.dataset.id = w.id;
  const frame = el('iframe');
  frame.setAttribute('sandbox', 'allow-scripts');
  const problem = el('div', 'cell-problem');
  problem.hidden = true;
  cell.append(frame, problem);

  // Edit mode: a cover over the widget to drag it, resize it or remove it.
  const cover = el('div', 'cell-edit');
  const head = el('div', 'cell-edit-head');
  head.append(el('span', 'cell-name'), el('span', 'size'));
  const change = (dw, dh) => {
    w.w = Math.max(1, Math.min(4, w.w + dw));
    w.h = Math.max(1, Math.min(4, w.h + dh));
    fitGrid();
    save();
  };
  const move = d => {
    const i = layout.indexOf(w), j = i + d;
    if (j < 0 || j >= layout.length) return;
    [layout[i], layout[j]] = [layout[j], layout[i]];
    fitGrid();
    save();
  };
  const tools = el('div', 'cell-tools');
  const stepper = (label, dec, inc) => {
    const s = el('div', 'stepper');
    s.append(button('−', 'step', dec), el('span', null, label), button('+', 'step', inc));
    s.firstChild.setAttribute('aria-label', 'Smaller ' + label.toLowerCase());
    s.lastChild.setAttribute('aria-label', 'Bigger ' + label.toLowerCase());
    return s;
  };
  const back = button('‹', 'step', () => move(-1));
  back.setAttribute('aria-label', 'Move earlier');
  const on = button('›', 'step', () => move(1));
  on.setAttribute('aria-label', 'Move later');
  const remove = button('Remove', 'btn small destructive', () => {
    layout = layout.filter(x => x !== w);
    renderHome();
    save();
  });
  tools.append(stepper('Width', () => change(-1, 0), () => change(1, 0)),
    stepper('Height', () => change(0, -1), () => change(0, 1)), back, on, remove);
  cover.append(head, tools);
  cell.appendChild(cover);

  // Dragging a cover over another widget puts it there.
  cell.draggable = editing;
  let before = '';
  cell.addEventListener('dragstart', e => {
    if (!editing) return e.preventDefault();
    cell.classList.add('dragging');
    before = layout.map(x => x.id).join();
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', w.id);
  });
  cell.addEventListener('dragend', () => {
    cell.classList.remove('dragging');
    if (layout.map(x => x.id).join() !== before) save();
  });
  cell.addEventListener('dragover', e => {
    const dragged = layout.find(x => x.id === grid.querySelector('.cell.dragging')?.dataset.id);
    if (!dragged || dragged === w) return;
    e.preventDefault();
    const r = cell.getBoundingClientRect();
    const after = e.clientX > r.left + r.width / 2;
    const rest = layout.filter(x => x !== dragged);
    rest.splice(rest.indexOf(w) + (after ? 1 : 0), 0, dragged);
    if (rest.some((x, i) => x !== layout[i])) {
      layout = rest;
      fitGrid();
    }
  });
  cell.addEventListener('drop', e => e.preventDefault()); // dragend saves it
  loadWidget(w, cell);
  return cell;
}

function setEditing(on) {
  editing = on;
  document.body.classList.toggle('home-editing', on);
  const b = $('home-edit');
  b.textContent = on ? 'Done' : 'Edit';
  b.setAttribute('aria-pressed', String(on));
  b.classList.toggle('primary', on);
  for (const cell of grid.children) cell.draggable = on;
}
$('home-edit').onclick = () => setEditing(!editing);

// Widgets' frames -------------------------------------------------------------------------------

// What every widget's page gets at the top of its <head>: fetched once.
let kit = null;
const widgetKit = () => (kit ??= Promise.all(['tokens.css', 'widgets/widget.css', 'widgets/sdk.js']
  .map(f => fetch(f).then(r => (r.ok ? r.text() : Promise.reject(new Error(f + ': ' + r.status)))))));

// No network, inline scripts and styles only: a widget's data comes through aex.call.
const widgetCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:";

// A placement's settings (HomeWidget.settings), at most this big as JSON (home.go: maxSettings).
const maxSettings = 8 << 10;

// widgetDoc is a widget's page with the kit, its placement's settings and the window's theme put in.
function widgetDoc(html, [tokens, base, sdk], settings) {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const csp = doc.createElement('meta');
  csp.httpEquiv = 'Content-Security-Policy';
  csp.content = widgetCSP;
  const style = doc.createElement('style');
  style.textContent = tokens + '\n' + base;
  const script = doc.createElement('script');
  // No "<" in the JSON, so nothing in it can end the script.
  script.textContent = 'const aexSettings = ' + JSON.stringify(settings || {}).replace(/</g, '\\u003c') + ';\n' + sdk;
  doc.head.prepend(csp, style, script);
  doc.documentElement.dataset.theme = document.documentElement.dataset.theme;
  return '<!doctype html>\n' + doc.documentElement.outerHTML;
}

// showProblem puts why a widget cannot show in its cell, with a way to fix it when there is one.
function showProblem(cell, text, plugin) {
  const box = cell.querySelector('.cell-problem');
  box.textContent = '';
  box.append(el('div', 'cell-problem-name', widgetName(layout.find(x => x.id === cell.dataset.id)?.widget || '')),
    el('div', 'cell-problem-text', text.charAt(0).toUpperCase() + text.slice(1) + '.'));
  if (plugin) {
    box.append(button('Review ' + plugin, 'btn small primary', () => runLine('plugins allow ' + plugin)));
  }
  box.hidden = false;
  cell.classList.add('broken');
  cell.querySelector('iframe').hidden = true;
}

// loadWidget loads (or reloads) the widget's page into its frame.
async function loadWidget(w, cell) {
  const frame = cell.querySelector('iframe');
  let page;
  try {
    const [p, k] = await Promise.all([api().WidgetPage(w.widget), widgetKit()]);
    if (p.problem) return showProblem(cell, p.problem, p.plugin);
    page = widgetDoc(p.html, k, w.settings);
  } catch (e) {
    return showProblem(cell, 'could not load: ' + e);
  }
  cell.classList.remove('broken');
  cell.querySelector('.cell-problem').hidden = true;
  frame.hidden = false;
  // A second load without a new page is the widget leaving it (a link, location): not allowed.
  frame.dataset.expect = '1';
  frame.onload = () => {
    if (frame.dataset.expect) delete frame.dataset.expect;
    else showProblem(cell, 'the widget left its page');
  };
  frame.srcdoc = page;
  w.refreshed = Date.now();
}

const post = (win, msg) => win.postMessage({ aex: 1, ...msg }, '*');

// Messages from widgets: only from a frame of the grid, which says which widget it is.
window.addEventListener('message', async e => {
  const m = e.data;
  if (!m || m.aex !== 1 || typeof m.id !== 'number') return;
  const cell = [...grid.children].find(c => c.querySelector('iframe').contentWindow === e.source);
  const w = cell && layout.find(x => x.id === cell.dataset.id);
  if (!w) return;
  const reply = (ok, value, error) => post(e.source, { op: 'reply', id: m.id, ok, value, error });
  if (m.op === 'call') {
    try {
      const args = m.args && typeof m.args === 'object' && !Array.isArray(m.args) ? m.args : {};
      reply(true, await api().WidgetCall(w.widget, String(m.name), args));
    } catch (err) {
      reply(false, undefined, String(err));
    }
  } else if (m.op === 'run') {
    const line = String(m.line || '').trim();
    const plugin = pluginOf(w.widget);
    if (plugin && splitArgs(line)[0] !== plugin) return reply(false, undefined, `a widget of ${plugin} runs only ${plugin}'s tools`);
    if (run) return reply(false, undefined, 'a tool is already running');
    runLine(line);
    reply(true);
  } else if (m.op === 'settings') {
    const s = m.settings;
    if (!s || typeof s !== 'object' || Array.isArray(s)) return reply(false, undefined, 'settings must be an object');
    if (JSON.stringify(s).length > maxSettings) return reply(false, undefined, `settings are over ${maxSettings >> 10} KB`);
    w.settings = s;
    try {
      await api().SaveHome(homeLayout());
      reply(true);
    } catch (err) {
      reply(false, undefined, String(err));
    }
  }
});

// The theme follows the window's.
new MutationObserver(() => {
  for (const f of grid.querySelectorAll('iframe')) {
    if (f.contentWindow) post(f.contentWindow, { op: 'theme', theme: document.documentElement.dataset.theme });
  }
}).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });

// A plugin's widget refreshes at most this often on its own: each of its calls starts the plugin.
const pluginRefreshMs = 30000;

// refreshWidget tells the widget to load its data again (sdk.js: aex.onRefresh); one that could not
// load (e.g. a plugin not approved) tries again.
function refreshWidget(w) {
  const cell = cellOf(w);
  if (!cell) return;
  if (cell.classList.contains('broken')) return loadWidget(w, cell);
  w.refreshed = Date.now();
  const f = cell.querySelector('iframe');
  if (f.contentWindow) post(f.contentWindow, { op: 'refresh' });
}

// refreshWidgets refreshes every widget after a run that may have changed their data.
async function refreshWidgets() {
  await loadCatalog();
  for (const w of layout) {
    if (pluginOf(w.widget) && !cellOf(w)?.classList.contains('broken') && Date.now() - (w.refreshed || 0) < pluginRefreshMs) continue;
    refreshWidget(w);
  }
}

// Auto refresh: each widget that asks for it (WidgetInfo.refresh, in seconds) is refreshed that
// often while the switch on Home is on. Paused while Home is not shown or the window is minimised
// (the web view keeps running then); a widget that fell due meanwhile refreshes when it is back.
async function autoRefreshTick() {
  if (!autoRefresh || page !== 'home' || document.hidden) return;
  const now = Date.now();
  const due = layout.filter(w => {
    const every = info(w.widget)?.refresh;
    return every && now - (w.refreshed || 0) >= every * 1000;
  });
  if (!due.length) return;
  try {
    if (await window.runtime.WindowIsMinimised()) return;
  } catch {}
  due.forEach(refreshWidget);
}
setInterval(autoRefreshTick, 1000);

const autoSwitch = $('home-auto-refresh');
// The switch shows only while a widget on the page refreshes on its own.
const syncAutoSwitch = () => ($('home-auto').hidden = !layout.some(w => info(w.widget)?.refresh));
autoSwitch.onchange = () => {
  autoRefresh = autoSwitch.checked;
  save();
};

// Adding widgets --------------------------------------------------------------------------------

async function loadCatalog() {
  try {
    catalog = await api().Widgets();
  } catch (e) {
    showCommandError('Could not list the widgets: ' + e);
  }
  syncAutoSwitch();
  fitGrid();
}

function closePicker() {
  picker.hidden = true;
  $('home-add').setAttribute('aria-expanded', 'false');
}

function openPicker() {
  picker.textContent = '';
  for (const w of catalog.widgets) {
    const item = button('', 'picker-item', () => {
      closePicker();
      const placed = { id: newID(), widget: w.id, w: w.w, h: w.h };
      layout.push(placed);
      renderHome();
      save();
      cellOf(placed)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
    item.setAttribute('role', 'menuitem');
    const count = layout.filter(x => x.widget === w.id).length;
    const name = el('span', 'name', w.name);
    if (w.plugin) name.append(el('span', 'from', w.plugin));
    item.append(name, el('span', 'summary', w.summary));
    if (count) item.append(el('span', 'placed', count === 1 ? 'On the page' : `On the page ${count}×`));
    picker.appendChild(item);
  }
  // Plugins not approved cannot say which widgets they have.
  const inactive = catalog.inactive || [];
  if (inactive.length) {
    const note = el('div', 'picker-note');
    note.append(el('div', null, "Don't see the widget you need? It may be in a plugin that is not active yet:"));
    for (const p of inactive) {
      const row = el('div', 'picker-plugin');
      const text = el('span', 'text');
      text.append(el('span', 'name', p.name), el('span', 'state', p.state));
      row.append(text, button('Review', 'btn small', () => { closePicker(); runLine('plugins allow ' + p.name); }));
      note.appendChild(row);
    }
    picker.appendChild(note);
  }
  picker.hidden = false;
  $('home-add').setAttribute('aria-expanded', 'true');
  picker.querySelector('button')?.focus();
}

$('home-add').onclick = async e => {
  e.stopPropagation();
  if (!picker.hidden) return closePicker();
  await loadCatalog(); // plugins may have come or gone
  openPicker();
};
document.addEventListener('click', e => { if (!picker.hidden && !picker.contains(e.target)) closePicker(); });
document.addEventListener('keydown', e => {
  if (e.key !== 'Escape') return;
  if (!picker.hidden) { closePicker(); $('home-add').focus(); }
  else if (editing) setEditing(false);
});

// Start -----------------------------------------------------------------------------------------

(async () => {
  try {
    const home = await api().Home();
    layout = home.widgets || [];
    autoRefresh = !home.autoRefreshOff;
    autoSwitch.checked = autoRefresh;
  } catch (e) {
    showCommandError('Could not load the home page: ' + e);
  }
  renderHome();
  if (!pageChosen) showPage(layout.length ? 'home' : 'runs', true);
  loadCatalog(); // names of plugins' widgets; describing plugins takes a moment
})();
document.body.dataset.page = 'runs';
document.querySelector('.page-link[data-page="runs"]').setAttribute('aria-current', 'true');
