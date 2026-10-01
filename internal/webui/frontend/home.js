// Pages and the home page. The window has three pages: Runs (the feed, app.js), Settings
// (settings.js) and Home, a grid of widgets. It opens on Home when that has widgets, else on Runs.
//
// Each widget is a page of its own in a sandboxed frame (scripts only: no access to the window, no
// network). Its page comes from the backend (built-in: widgets/<id>.html; a plugin's: from the
// plugin), with the window's colors, widget.css and widgets/sdk.js put at the top of its <head>. It
// reaches the window only through messages, answered here: its data calls, running a tool and
// showing a reminder.
// Backend: home.go (Widgets, Home, SaveHome, WidgetPage, WidgetCall).
const grid = $('grid'), picker = $('widget-picker');

let page = 'runs';
let pageChosen = false; // once the user or a question picked a page, loading Home keeps it
const scrollTops = { home: 0, runs: 0, settings: 0 };
let catalog = { widgets: [], inactive: [] }; // WidgetList
let layout = [];        // HomeWidget: {id, widget, w, h, x, y, settings}, sorted by place (byPlace)
let autoRefresh = true; // !HomeLayout.autoRefreshOff
let editing = false;
let popup = null;       // a widget's popup (openPopup): {w, from: the cell's window, id: its request}
let cols = 12;
// Widths are in twelfths of the grid (home.go: gridColumns), heights in rows; as clamped in home.go.
const maxW = 12, maxH = 4;
const clamp = (n, lo, hi) => Math.max(lo, Math.min(hi, n));
// shownW is how many columns a widget takes now: all of its width, or the whole row when the
// window is too narrow for it.
const shownW = w => Math.min(w.w, cols);
// full: the grid is at its full width, where each widget is at its place (x, y: column and row of its
// top left corner, gaps allowed). Narrower, they follow one another in that order.
const full = () => cols === maxW;

// Places (as in home.go: placeWidgets).
const byPlace = (a, b) => a.y - b.y || a.x - b.x;
const overlaps = (a, b) => a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
const maxRow = 48 * maxH; // home.go: maxRow

// settle makes room for pinned (a widget just moved or resized, kept where it is): every other one
// that overlaps it, or one moved down before it, goes down to the first row where it fits. Then the
// layout is sorted by place.
function settle(pinned) {
  const placed = [];
  if (pinned) {
    pinned.x = clamp(pinned.x, 0, maxW - pinned.w);
    pinned.y = clamp(pinned.y, 0, maxRow);
    placed.push(pinned);
  }
  for (const w of layout.filter(x => x !== pinned).sort(byPlace)) {
    w.x = clamp(w.x, 0, maxW - w.w);
    while (placed.some(p => overlaps(p, w))) w.y++;
    placed.push(w);
  }
  layout.sort(byPlace);
}

// firstFit is the first place, row by row from the top left, where a widget of w×h fits.
function firstFit(w, h) {
  for (let y = 0; ; y++) {
    for (let x = 0; x + w <= maxW; x++) {
      if (!layout.some(o => overlaps(o, { x, y, w, h }))) return { x, y };
    }
  }
}

const placesOf = () => new Map(layout.map(w => [w.id, { x: w.x, y: w.y }]));
const samePlaces = (a, b) => layout.every(w => a.get(w.id)?.x === b.get(w.id)?.x && a.get(w.id)?.y === b.get(w.id)?.y);

// On Home the sidebar is hidden unless the user opened it there (remembered in this browser); on
// Runs it is always shown.
let homeSide = false;
try { homeSide = localStorage.getItem('aex.homeSide') === 'open'; } catch {}

function syncSide(animate = true) {
  const collapsed = page === 'home' && !homeSide;
  if (!animate) {
    document.body.classList.add('instant');
    requestAnimationFrame(() => requestAnimationFrame(() => document.body.classList.remove('instant')));
  }
  document.body.classList.toggle('side-collapsed', collapsed);
  $('side').inert = collapsed;
  const t = $('side-toggle');
  t.setAttribute('aria-expanded', String(!collapsed));
  t.title = (collapsed ? 'Show' : 'Hide') + ' sidebar (Ctrl+B)';
  t.setAttribute('aria-label', collapsed ? 'Show sidebar' : 'Hide sidebar');
}

function toggleSide() {
  if (page !== 'home') return;
  homeSide = !homeSide;
  try { localStorage.setItem('aex.homeSide', homeSide ? 'open' : 'closed'); } catch {}
  syncSide();
}
$('side-toggle').onclick = toggleSide;
document.addEventListener('keydown', e => {
  if (page === 'home' && e.ctrlKey && !e.altKey && !e.shiftKey && e.key.toLowerCase() === 'b') {
    e.preventDefault();
    toggleSide();
  }
});

// showPage shows Home, Runs or Settings. auto: chosen by the window at start, not by the user.
function showPage(name, auto = false) {
  if (!auto) pageChosen = true;
  if (name === page) return;
  if (page === 'settings') leavePluginSettings();
  scrollTops[page] = scroller.scrollTop;
  page = name;
  $('home').hidden = name !== 'home';
  $('page').hidden = name !== 'runs';
  $('settings').hidden = name !== 'settings';
  document.body.dataset.page = name;
  document.querySelectorAll('.page-link').forEach(b => b.setAttribute('aria-current', String(b.dataset.page === name)));
  if (name === 'runs') document.body.classList.remove('runs-activity', 'runs-problem');
  if (notice && notice.r.page === name) hideNotice();
  if (name !== 'home') { closePicker(); closePopup(); }
  scroller.scrollTop = scrollTops[name];
  syncSide(!auto);
  if (name === 'home') fitGrid();
  if (name === 'settings') onSettingsPage();
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

// glide runs apply (which moves, resizes or removes widgets) and slides the widgets that moved from
// where they were to where they are now.
function glide(apply) {
  if (reduceMotion() || page !== 'home') return apply();
  const cells = [...grid.children];
  const before = new Map(cells.map(c => [c, c.getBoundingClientRect()]));
  apply();
  for (const c of cells) {
    if (!c.isConnected) continue;
    const a = before.get(c), b = c.getBoundingClientRect();
    const dx = a.left - b.left, dy = a.top - b.top;
    if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue;
    c.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }], { duration: 280, easing: 'cubic-bezier(.2, .8, .2, 1)' });
  }
}

let firstRender = true; // the widgets Home opens with come in one after another

function renderHome() {
  $('home-empty').hidden = layout.length > 0;
  $('home-edit').hidden = !layout.length;
  syncAutoSwitch();
  if (!layout.length && editing) setEditing(false);
  const cells = new Map([...grid.children].map(c => [c.dataset.id, c]));
  for (const w of layout) {
    if (!cells.has(w.id)) {
      const cell = makeCell(w);
      if (firstRender) cell.style.setProperty('--i', grid.childElementCount);
      grid.appendChild(cell);
    }
    cells.delete(w.id);
  }
  if (layout.length) firstRender = false;
  cells.forEach(c => c.remove());
  if (popup && !layout.includes(popup.w)) closePopup();
  fitGrid();
}

// fitGrid sets the columns for the grid's width, and each widget's place in them: at full width its
// column and row, narrower its turn (CSS order: moving a frame in the page would reload it) and span.
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
  cols = colsFor(width);
  grid.style.setProperty('--cols', cols);
  // At full width the rows go down to the lowest widget; in Edit, a widget's height more to move one below.
  const rows = Math.max(0, ...layout.map(w => w.y + w.h)) + (editing ? maxH : 0);
  grid.style.gridTemplateRows = full() ? `repeat(${rows}, var(--row))` : '';
  grid.classList.toggle('placed', full());
  for (const w of layout) {
    const cell = cellOf(w);
    if (!cell) continue;
    cell.style.gridColumn = full() ? `${w.x + 1} / span ${w.w}` : `span ${shownW(w)}`;
    cell.style.gridRow = full() ? `${w.y + 1} / span ${w.h}` : `span ${w.h}`;
    cell.querySelector('.size').textContent = `${shownW(w)}/${cols} × ${w.h}`;
  }
}
function colsFor(width) { return width >= 900 ? 12 : width >= 440 ? 6 : 1; }
// The widgets glide into a new number of columns (the window resized, the sidebar slid).
new ResizeObserver(() => (grid.clientWidth && colsFor(grid.clientWidth) !== cols ? glide(fitGrid) : fitGrid())).observe(grid);

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
  // Widths change from what is shown: in a narrow window a widget is at most as wide as it is. At
  // full width it grows up to the right edge. Widgets it grows over go down.
  const change = (dw, dh) => {
    if (dw) w.w = clamp(shownW(w) + dw, 1, full() ? maxW - w.x : cols);
    w.h = clamp(w.h + dh, 1, maxH);
    settle(w);
    glide(fitGrid);
    save();
  };
  // A widget moved onto others pushes them down.
  const move = (dx, dy) => {
    const x = clamp(w.x + dx, 0, maxW - w.w), y = clamp(w.y + dy, 0, maxRow);
    if (x === w.x && y === w.y) return;
    w.x = x;
    w.y = y;
    settle(w);
    glide(fitGrid);
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
  const moves = el('div', 'stepper');
  for (const [text, label, dx, dy] of [['←', 'left', -1, 0], ['↑', 'up', 0, -1], ['↓', 'down', 0, 1], ['→', 'right', 1, 0]]) {
    const b = button(text, 'step', () => move(dx, dy));
    b.setAttribute('aria-label', 'Move ' + label);
    b.title = 'Move ' + label;
    moves.appendChild(b);
  }
  const remove = button('Remove', 'btn small destructive', async () => {
    cell.style.pointerEvents = 'none';
    if (!reduceMotion()) {
      await cell.animate([{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.92)' }],
        { duration: 170, easing: 'ease-in', fill: 'forwards' }).finished;
    }
    layout = layout.filter(x => x !== w);
    glide(renderHome);
    save();
  });
  tools.append(stepper('Width', () => change(-1, 0), () => change(1, 0)),
    stepper('Height', () => change(0, -1), () => change(0, 1)), moves, remove);
  cover.append(head, tools, resizer(cell, w, 'e'), resizer(cell, w, 's'), resizer(cell, w, 'se'));
  cell.appendChild(cover);

  // Dragging a cover moves the widget (the grid's dragover, below).
  cell.draggable = editing;
  cell.addEventListener('dragstart', e => {
    if (!editing || cell.classList.contains('resizing')) return e.preventDefault();
    cell.classList.add('dragging');
    const r = cell.getBoundingClientRect();
    drag = { w, from: placesOf(), dx: e.clientX - r.left, dy: e.clientY - r.top, at: `${w.x},${w.y}` };
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', w.id);
  });
  cell.addEventListener('dragend', e => {
    cell.classList.remove('dragging');
    const d = drag;
    drag = null;
    if (!d) return;
    if (e.dataTransfer.dropEffect === 'none') { // let go outside the grid, or Esc: back where they were
      restore(d.from);
      settle();
      glide(fitGrid);
    } else if (!samePlaces(d.from, placesOf())) {
      save();
    }
    fitGrid(); // drops the rows kept below the lowest widget while dragging
  });
  loadWidget(w, cell);
  return cell;
}

// drag is the widget being dragged: where every widget was when it started (from), where on the
// widget it was picked up, in pixels (dx, dy), and where it is now (at).
let drag = null;

function restore(from) {
  for (const w of layout) {
    const p = from.get(w.id);
    if (p) Object.assign(w, p);
  }
}

// gridSteps is the width of a column and the height of a row in pixels, each with its gap.
function gridSteps() {
  const style = getComputedStyle(grid);
  const colGap = parseFloat(style.columnGap) || 0, rowGap = parseFloat(style.rowGap) || 0;
  return {
    col: (grid.clientWidth - colGap * (cols - 1)) / cols + colGap,
    row: (parseFloat(style.getPropertyValue('--row')) || 150) + rowGap,
  };
}

// Over the grid, a dragged widget goes where it would be dropped: any free place, or onto others,
// which go down to make room. Nothing is saved until it is let go. In a narrow window, where the
// widgets follow one another, it trades places with the widget it is over.
grid.addEventListener('dragover', e => {
  if (!drag) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  const w = drag.w;
  let to, over;
  if (full()) {
    const g = grid.getBoundingClientRect(), step = gridSteps();
    to = {
      x: clamp(Math.round((e.clientX - drag.dx - g.left) / step.col), 0, maxW - w.w),
      y: clamp(Math.round((e.clientY - drag.dy - g.top) / step.row), 0, maxRow),
    };
  } else {
    over = layout.find(x => x.id === e.target.closest?.('.cell')?.dataset.id);
    if (!over || over === w) return;
    to = drag.from.get(over.id);
  }
  if (`${to.x},${to.y}` === drag.at) return;
  drag.at = `${to.x},${to.y}`;
  restore(drag.from);
  if (over) Object.assign(over, drag.from.get(w.id));
  Object.assign(w, to);
  settle(w);
  glide(fitGrid);
});
grid.addEventListener('drop', e => e.preventDefault()); // dragend saves it

// resizer is a handle on a widget's cover, on its right edge (dir 'e'), bottom edge ('s') or corner
// ('se'): dragging it resizes the widget a column or a row at a time, saved when let go.
function resizer(cell, w, dir) {
  const handle = el('div', 'cell-resize ' + dir);
  handle.setAttribute('aria-hidden', 'true'); // the Width and Height steppers do it from the keyboard
  handle.addEventListener('pointerdown', e => {
    if (!editing || e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    handle.setPointerCapture(e.pointerId);
    cell.draggable = false;
    cell.classList.add('resizing');
    const { col: colStep, row: rowStep } = gridSteps();
    // From where it started, not where the widget is now: a resized widget can move in the grid.
    // Widgets it grows over go down, and come back as it shrinks again.
    const x0 = e.clientX, y0 = e.clientY, w0 = shownW(w), h0 = w.h, before = `${w.w}x${w.h}`, from = placesOf();
    const move = ev => {
      const nw = dir === 's' ? w.w : clamp(w0 + Math.round((ev.clientX - x0) / colStep), 1, full() ? maxW - w.x : cols);
      const nh = dir === 'e' ? w.h : clamp(h0 + Math.round((ev.clientY - y0) / rowStep), 1, maxH);
      if (nw === w.w && nh === w.h) return;
      w.w = nw;
      w.h = nh;
      restore(from);
      settle(w);
      glide(fitGrid);
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('pointercancel', end);
      cell.classList.remove('resizing');
      cell.draggable = editing;
      if (`${w.w}x${w.h}` !== before || !samePlaces(from, placesOf())) save();
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  });
  return handle;
}

function setEditing(on) {
  editing = on;
  document.body.classList.toggle('home-editing', on);
  const b = $('home-edit');
  b.textContent = on ? 'Done' : 'Edit';
  b.setAttribute('aria-pressed', String(on));
  b.classList.toggle('primary', on);
  for (const cell of grid.children) cell.draggable = on;
  fitGrid(); // in Edit, room below the lowest widget to move one there
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
// popup: {data} for the page in a popup (openPopup), null in its cell.
function widgetDoc(html, [tokens, base, sdk], settings, popup = null) {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const csp = doc.createElement('meta');
  csp.httpEquiv = 'Content-Security-Policy';
  csp.content = widgetCSP;
  const style = doc.createElement('style');
  style.textContent = tokens + '\n' + base;
  const script = doc.createElement('script');
  // No "<" in the JSON, so nothing in it can end the script.
  const json = v => JSON.stringify(v).replace(/</g, '\\u003c');
  script.textContent = 'const aexSettings = ' + json(settings || {}) + ';\nconst aexPopup = ' + json(popup) + ';\n' + sdk;
  doc.head.prepend(csp, style, script);
  doc.documentElement.dataset.theme = theme.scheme();
  for (const [k, v] of Object.entries(theme.vars())) doc.documentElement.style.setProperty(k, v);
  return '<!doctype html>\n' + doc.documentElement.outerHTML;
}

// showProblem puts why a widget cannot show in its cell, with a way to fix it when there is one.
// An unknown widget (one aex does not have) shows as a warning.
function showProblem(cell, text, plugin, unknown) {
  const box = cell.querySelector('.cell-problem');
  box.textContent = '';
  const name = el('div', 'cell-problem-name');
  if (unknown) {
    const icon = el('span', 'warn-icon');
    icon.innerHTML = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1.5 15 14H1z"/><path class="mark" d="M8 6v3.5M8 11.6v.1"/></svg>';
    icon.setAttribute('role', 'img');
    icon.setAttribute('aria-label', 'Warning');
    name.appendChild(icon);
  }
  name.append(widgetName(layout.find(x => x.id === cell.dataset.id)?.widget || ''));
  box.classList.toggle('unknown', !!unknown);
  box.append(name, el('div', 'cell-problem-text', text.charAt(0).toUpperCase() + text.slice(1) + '.'));
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
    if (p.problem) return showProblem(cell, p.problem, p.plugin, p.unknown);
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

// framesOf are the windows of a widget's pages: its cell's and its popup's, when open.
function framesOf(w) {
  const wins = [cellOf(w)?.querySelector('iframe').contentWindow];
  if (popup?.w === w) wins.push(popupFrame.contentWindow);
  return wins.filter(Boolean);
}

// Messages from widgets: only from a frame of the grid or the popup, which says which widget it is.
window.addEventListener('message', async e => {
  const m = e.data;
  if (!m || m.aex !== 1 || typeof m.id !== 'number') return;
  const inPopup = !!popup && e.source === popupFrame.contentWindow;
  const cell = !inPopup && [...grid.children].find(c => c.querySelector('iframe').contentWindow === e.source);
  const w = inPopup ? popup.w : cell && layout.find(x => x.id === cell.dataset.id);
  if (!w) return;
  const reply = (ok, value, error) => post(e.source, { op: 'reply', id: m.id, ok, value, error });
  if (inPopup && m.op === 'close') return closePopup(m.value);
  if (inPopup && m.op === 'size') return sizePopup(m.height);
  if (!inPopup && m.op === 'popup') return openPopup(w, e.source, m.id, m.opts);
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
    runLine(line, { home: !!m.home });
    reply(true);
  } else if (m.op === 'remind') {
    try {
      reply(true, await api().Remind(String(m.title || ''), String(m.message || ''), !!m.urgent));
    } catch (err) {
      reply(false, undefined, String(err));
    }
  } else if (m.op === 'settings') {
    const s = m.settings;
    if (!s || typeof s !== 'object' || Array.isArray(s)) return reply(false, undefined, 'settings must be an object');
    if (JSON.stringify(s).length > maxSettings) return reply(false, undefined, `settings are over ${maxSettings >> 10} KB`);
    w.settings = s;
    // The widget's other page (cell or popup) gets them too.
    for (const win of framesOf(w)) if (win !== e.source) post(win, { op: 'settings', settings: s });
    try {
      await api().SaveHome(homeLayout());
      reply(true);
    } catch (err) {
      reply(false, undefined, String(err));
    }
  }
});

// Popups -----------------------------------------------------------------------------------------

// A widget can open its page again in a popup over the window (sdk.js: aex.openPopup), for what does
// not fit in its cell, such as its settings. One at a time. It closes with aex.closePopup(value) in
// it, the ×, Esc, a click outside, leaving Home or the widget being removed; the widget's openPopup
// gets value (undefined but for aex.closePopup).
const popupBack = $('widget-popup'), popupBox = popupBack.querySelector('.popup'), popupFrame = $('popup-frame');

async function openPopup(w, from, id, opts) {
  const fail = error => post(from, { op: 'reply', id, ok: false, error });
  if (popup) return fail('a popup is already open');
  if (page !== 'home') return fail('Home is not shown');
  opts = opts && typeof opts === 'object' ? opts : {};
  let doc;
  try {
    const [p, k] = await Promise.all([api().WidgetPage(w.widget), widgetKit()]);
    if (p.problem) return fail(p.problem);
    doc = widgetDoc(p.html, k, w.settings, { data: opts.data ?? null });
  } catch (err) {
    return fail('could not load: ' + err);
  }
  if (popup) return fail('a popup is already open');
  popup = { w, from, id };
  $('popup-title').textContent = String(opts.title || widgetName(w.widget)).slice(0, 120);
  popupBox.style.width = clamp(Number(opts.width) || 480, 240, 1200) + 'px';
  popupFrame.style.height = '';
  popupFrame.title = widgetName(w.widget);
  // As in a cell: a second load is the page leaving (a link, location), which closes it.
  let loaded = false;
  popupFrame.onload = () => {
    if (!loaded) { loaded = true; popupFrame.focus(); } else closePopup();
  };
  popupFrame.srcdoc = doc;
  popupBack.hidden = false;
}

// sizePopup makes the popup as tall as its page (the CSS caps it at the window's height).
function sizePopup(height) {
  const h = Number(height);
  if (h > 0) popupFrame.style.height = Math.ceil(h) + 'px';
}

function closePopup(value) {
  if (!popup) return;
  const { from, id } = popup;
  popup = null;
  popupBack.hidden = true;
  popupFrame.onload = null;
  popupFrame.removeAttribute('srcdoc');
  popupFrame.src = 'about:blank';
  post(from, { op: 'reply', id, ok: true, value: value ?? undefined });
}

$('popup-close').onclick = () => closePopup();
popupBack.addEventListener('mousedown', e => { if (e.target === popupBack) closePopup(); });

// The theme follows the window's: its mode and colors (theme.js).
window.addEventListener('aex:theme', () => {
  const m = { op: 'theme', theme: theme.scheme(), vars: theme.vars() };
  if (popup && popupFrame.contentWindow) post(popupFrame.contentWindow, m);
  for (const f of grid.querySelectorAll('iframe')) {
    if (f.contentWindow) post(f.contentWindow, m);
  }
});

// A plugin's widget refreshes at most this often on its own: each of its calls starts the plugin.
const pluginRefreshMs = 30000;

// refreshWidget tells the widget to load its data again (sdk.js: aex.onRefresh); one that could not
// load (e.g. a plugin not approved) tries again.
function refreshWidget(w) {
  const cell = cellOf(w);
  if (!cell) return;
  if (cell.classList.contains('broken')) return loadWidget(w, cell);
  w.refreshed = Date.now();
  for (const win of framesOf(w)) post(win, { op: 'refresh' });
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
      const placed = { id: newID(), widget: w.id, w: w.w, h: w.h, ...firstFit(w.w, w.h) };
      if (w.settings) placed.settings = w.settings;
      layout.push(placed);
      layout.sort(byPlace);
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
  if (popup) closePopup();
  else if (!picker.hidden) { closePicker(); $('home-add').focus(); }
  else if (editing) setEditing(false);
});

// Start -----------------------------------------------------------------------------------------

(async () => {
  try {
    const home = await api().Home();
    layout = home.widgets || [];
    for (const w of layout) if (w.x == null || w.y == null) Object.assign(w, firstFit(w.w, w.h)); // home.go places them all
    layout.sort(byPlace);
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
