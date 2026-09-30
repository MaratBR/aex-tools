// About, a section of the Settings page (settings.js): aex's version and build, the plugins
// pre-approved in it, its license and the licenses of everything built into it. Backend: about.go
// (About); the licenses come from internal/about/about.json, gathered from the source.

// link is a link opened in the browser (not in the window).
function link(url, text) {
  const a = el('a', null, text || url.replace(/^https?:\/\//, ''));
  a.href = url;
  a.onclick = e => { e.preventDefault(); window.runtime.BrowserOpenURL(url); };
  return a;
}

// aboutRow is a row of a card: label, then value (text or nodes).
function aboutRow(label, ...value) {
  const row = el('div', 'row about-row');
  const v = el('span', 'about-value');
  v.append(...value);
  row.append(el('span', 'about-label', label), v);
  return row;
}

// licenseText is a license file, shown when its summary is clicked.
function licenseText(summary, text, open = false) {
  const d = el('details', 'about-text');
  d.open = open;
  d.append(el('summary', null, summary), el('pre', null, text));
  return d;
}

const fullDate = s => new Date(s).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });

// aboutSection adds a titled group to About and returns its card.
function aboutSection(title, foot) {
  const g = el('section', 'group');
  g.appendChild(el('h3', null, title));
  const card = el('div', 'card');
  g.appendChild(card);
  if (foot) g.appendChild(el('p', 'group-foot', foot));
  aboutPane.appendChild(g);
  return card;
}

let aboutLoaded = false;
let aboutBehind = null; // the Updates row of Build
let checkingBehind = false;

// renderAboutBehind fills the Updates row from the last check (behind, app.js).
function renderAboutBehind() {
  if (!aboutBehind) return;
  const v = aboutBehind.querySelector('.about-value');
  v.textContent = '';
  if (!behind) {
    v.append(el('span', 'dim', 'Not checked yet'));
  } else {
    const { state, text } = behindText(behind);
    v.append(el('span', state === 'in' ? null : 'about-warn', text));
    if (behind.compareURL && behind.behind) v.append(' ', link(behind.compareURL, 'see what changed'));
    v.append(el('span', 'dim', ` · checked ${fullDate(behind.checkedAt)} `));
  }
  const btn = el('button', 'btn small', checkingBehind ? 'Checking…' : 'Check now');
  btn.type = 'button';
  btn.disabled = checkingBehind;
  btn.title = 'Ask GitHub now (at most once a minute; else every few hours)';
  btn.onclick = async () => {
    checkingBehind = true;
    renderAboutBehind();
    try { await loadBehind(true); } finally { checkingBehind = false; renderAboutBehind(); }
  };
  v.append(btn);
}

async function loadAbout() {
  if (aboutLoaded) return;
  aboutPane.textContent = '';
  let info;
  try {
    info = await api().About();
  } catch (e) {
    aboutPane.appendChild(el('p', 'settings-note error', String(e)));
    return;
  }
  aboutLoaded = true;
  const { build: b, licenses: l } = info;

  const head = el('div', 'about-head');
  const logo = el('img');
  logo.src = 'logo.svg';
  logo.alt = '';
  const title = el('div');
  title.append(el('h2', null, 'aex tools'), el('div', 'dim', `Version ${b.version}${info.dev ? ' (dev build)' : ''}`));
  head.append(logo, title);
  aboutPane.appendChild(head);

  const card = aboutSection('Build');
  card.append(aboutRow('Built', b.builtAt ? fullDate(b.builtAt) + (b.builtAtFile ? ' (the exe file\'s time)' : '') : 'Not recorded'));
  if (b.commit) {
    const commit = [link(b.commitURL, b.commit.slice(0, 12))];
    if (b.commitTime) commit.push(el('span', 'dim', ` of ${fullDate(b.commitTime)}`));
    if (b.modified) commit.push(el('span', 'about-warn', ' plus changes not committed'));
    card.append(aboutRow('Commit', ...commit));
  } else card.append(aboutRow('Commit', el('span', 'dim', 'Not recorded (built without git info, e.g. go run)')));
  card.append(aboutRow('Source', link(b.repo)));
  if (b.commit) {
    aboutBehind = aboutRow('Updates');
    card.append(aboutBehind);
    renderAboutBehind();
  }
  card.append(aboutRow('Go', b.go));
  card.append(aboutRow('Platform', b.platform));

  const pre = aboutSection('Pre-approved plugins',
    'Plugins built together with this exe: a plugin file with one of these SHA-256 hashes runs without asking to approve it.');
  if (!info.preApproved?.length) pre.append(aboutRow('None', el('span', 'dim', 'This exe was built without its plugins\' hashes.')));
  for (const p of info.preApproved || []) {
    const row = el('div', 'row about-hash');
    row.append(el('code', null, p.hash), el('span', p.file ? 'about-file' : 'about-file dim', p.file || 'no file with it in the plugins folder'));
    pre.append(row);
  }

  const lic = aboutSection('License');
  const own = el('div', 'about-license');
  own.append(el('p', null, `aex tools is licensed under the Apache License 2.0 (${l.license}).`),
    licenseText('LICENSE', l.text), licenseText('NOTICE', l.notice, true));
  lic.append(own);

  const deps = aboutSection('Open source',
    `aex includes these ${l.thirdParty.length} components, under their own licenses. The list is gathered from the source when aex is built: every Go module linked on Windows, macOS and Linux, and the files vendored or built in.`);
  for (const c of l.thirdParty) {
    const d = el('details', 'about-dep');
    const s = el('summary');
    s.append(el('span', 'about-dep-name', c.name), el('span', 'about-dep-version', c.version || ''), el('span', 'about-dep-license', c.license));
    const body = el('div', 'about-dep-body');
    body.append(link(c.url));
    if (c.note) body.append(el('div', 'field-hint', c.note));
    for (const f of c.files || []) body.append(licenseText(f.path, f.text));
    d.append(s, body);
    deps.append(d);
  }
}
