// What a widget gets from the window. A widget's page runs in a sandboxed frame of the home page
// (home.js), with no access to the window and no network: home.js puts the window's colors
// (tokens.css), widget.css and this file at the top of its <head>, and answers its messages.
// It gets `aex`:
//   aex.call(name, args)  one of its data calls, a Promise: built-in widgets call the widget APIs in
//                         home.go (widgetAPIs), e.g. aex.call('quota'); a plugin's widget its
//                         plugin's calls (plugin.Widget.Calls). Rejects with an Error.
//   aex.run(line)         runs a tool on the Runs page, as if typed on the command line; a plugin's
//                         widget only its plugin's tools
//   aex.onRefresh(fn)     fn runs when the data may have changed (after a run) and on aex.refresh()
// The theme follows the window's.
const aex = (() => {
  const pending = new Map();
  let seq = 0;
  addEventListener('message', e => {
    const m = e.data;
    if (e.source !== parent || !m || m.aex !== 1) return;
    if (m.op === 'reply') {
      const p = pending.get(m.id);
      if (!p) return;
      pending.delete(m.id);
      if (m.ok) p.resolve(m.value);
      else p.reject(new Error(m.error || 'failed'));
    } else if (m.op === 'theme') {
      document.documentElement.dataset.theme = m.theme;
    } else if (m.op === 'refresh') {
      dispatchEvent(new Event('aex:refresh'));
    }
  });
  const request = (op, fields) => new Promise((resolve, reject) => {
    const id = ++seq;
    pending.set(id, { resolve, reject });
    parent.postMessage({ aex: 1, op, id, ...fields }, '*');
  });
  return Object.freeze({
    call: (name, args = {}) => request('call', { name, args }),
    run: line => request('run', { line }),
    onRefresh: fn => addEventListener('aex:refresh', () => fn()),
    refresh: () => dispatchEvent(new Event('aex:refresh')),
  });
})();
