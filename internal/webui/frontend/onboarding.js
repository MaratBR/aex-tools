// Onboarding: a screen over the whole window that asks to log in to each service the header has a
// login for (gui.go: the Header lines with a state), each with account --login <service>. Opened
// on first start (onboarding.go: NeedsOnboarding) before the start-up questions, and by the debug
// tool onboarding. A login runs on Runs like any tool, its questions asked there: the screen steps
// aside for it and comes back when it ends.
const onboarding = $('onboarding');
let onboardingClosed = null;  // {promise, resolve} while the screen is open
let onboardingLogin = null;   // the service a login started here is for, while it runs
const onboardingNotes = {};   // service: why its last login from here did not finish
let onboardingSeq = 0;

// What each service is for; google cannot log in while its OAuth client is not set (unset).
const obServices = {
  aext: { about: 'Worklogs and the hours quota' },
  jira: { about: 'Tickets, for worklogs and the Jira tickets widget' },
  google: { about: 'Calendar, for the Google Calendar now widget', needsSetup: true },
};

// openOnboarding shows the screen; it resolves once the user is done with it.
function openOnboarding() {
  if (!onboardingClosed) {
    let resolve;
    const promise = new Promise(r => (resolve = r));
    onboardingClosed = { promise, resolve };
  }
  showOnboarding();
  $('ob-done').focus();
  return onboardingClosed.promise;
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
    await api().Onboarded();
  } catch (e) {
    showCommandError('Could not save that onboarding is done: ' + e);
  }
  hideOnboarding();
  const closed = onboardingClosed;
  onboardingClosed = null;
  closed?.resolve();
};
