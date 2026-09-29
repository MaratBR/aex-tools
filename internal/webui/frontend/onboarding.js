// Onboarding: a screen over the whole window that asks to log in to each service the header has a
// login for (gui.go: the Header lines with a state), each with account --login <service>. Opened
// on first start (onboarding.go: NeedsOnboarding) before the start-up questions, and by the debug
// tool onboarding. It also asks whether aex starts when the user logs in, and on which days
// (settings.js: autostartRows), saved on Continue. A login runs on Runs like any tool, its questions asked there: the screen steps
// aside for it and comes back when it ends.
const onboarding = $('onboarding');
let onboardingClosed = null;  // {promise, resolve} while the screen is open
let onboardingLogin = null;   // the service a login started here is for, while it runs
const onboardingNotes = {};   // service: why its last login from here did not finish
let onboardingSeq = 0;
let onboardingAutostart = null; // {enabled, days} to save on Continue; null where aex cannot start with the system

// What each service is for; google cannot log in while its OAuth client is not set (unset).
const obServices = {
  aext: { about: 'Worklogs and the hours quota' },
  jira: { about: 'Tickets, for worklogs and the Jira tickets widget' },
  google: { about: 'Calendar, for the Google Calendar widget', needsSetup: true },
};

// openOnboarding shows the screen; it resolves once the user is done with it.
function openOnboarding() {
  if (!onboardingClosed) {
    let resolve;
    const promise = new Promise(r => (resolve = r));
    onboardingClosed = { promise, resolve };
  }
  showOnboarding();
  loadAutostart();
  $('ob-done').focus();
  return onboardingClosed.promise;
}

// loadAutostart shows whether aex starts when the user logs in: on workdays unless already set up
// (a release build only; a dev build keeps it off).
async function loadAutostart() {
  const box = $('ob-autostart');
  const card = box.querySelector('.card');
  card.textContent = '';
  onboardingAutostart = null;
  const info = await api().Autostart().catch(() => null);
  box.hidden = !info;
  if (!info) return;
  if (!info.enabled && info.suggested) info.enabled = true;
  onboardingAutostart = { enabled: info.enabled, days: info.days };
  card.append(...autostartRows(info, v => { onboardingAutostart = v; }));
}

function showOnboarding() {
  onboarding.hidden = false;
  document.body.classList.add('onboarding');
  loadServices();
}

function hideOnboarding() {
  onboarding.hidden = true;
  document.body.classList.remove('onboarding');
}

// resumeOnboarding brings the screen back after a login started from it (app.js: finished).
function resumeOnboarding(ok, status) {
  const service = onboardingLogin;
  if (!service) return;
  onboardingLogin = null;
  if (ok) delete onboardingNotes[service];
  else onboardingNotes[service] = String(status || 'Did not finish').replace(/^✖\s*/, '');
  showOnboarding();
}

async function loadServices() {
  const n = ++onboardingSeq;
  try {
    renderServices(await api().Header(false), n);
    renderServices(await api().Header(true), n);
  } catch (e) {
    $('ob-services').textContent = 'Could not check the logins: ' + e;
  }
}

function renderServices(lines, n) {
  if (n !== onboardingSeq) return;
  const box = $('ob-services');
  box.textContent = '';
  for (const l of lines.filter(l => l.state)) {
    const service = l.label.toLowerCase();
    const known = obServices[service] || {};
    const row = el('div', 'ob-service ' + l.state);
    const what = el('div', 'what');
    what.append(el('div', 'name', l.label));
    if (known.about) what.append(el('div', 'about', known.about));
    const state = el('div', 'state');
    for (const s of l.segments || []) state.appendChild(el('span', s.kind, s.text));
    what.append(state);
    if (onboardingNotes[service]) what.append(el('div', 'note', onboardingNotes[service]));
    row.append(what);
    if (!(known.needsSetup && l.state === 'unset')) {
      const label = l.state === 'in' ? 'Log in again' : l.state === 'unset' ? 'Set up' : 'Log in';
      const b = button(label, 'btn' + (l.state === 'in' ? '' : ' primary'), () => login(service));
      b.disabled = l.state === 'checking';
      row.append(b);
    }
    box.appendChild(row);
  }
}

function login(service) {
  if (run) return showCommandError('A tool is already running.');
  onboardingLogin = service;
  hideOnboarding();
  runLine('account --login ' + service);
}

$('ob-done').onclick = async () => {
  try {
    if (onboardingAutostart) await api().SetAutostart(onboardingAutostart.enabled, onboardingAutostart.days);
  } catch (e) {
    showCommandError('Could not set aex to start when you log in: ' + e);
  }
  try {
    await api().Onboarded();
  } catch (e) {
    showCommandError('Could not save that onboarding is done: ' + e);
  }
  hideOnboarding();
  const closed = onboardingClosed;
  onboardingClosed = null;
  closed?.resolve();
};
