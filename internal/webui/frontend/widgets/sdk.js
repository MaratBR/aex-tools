// What a widget gets from the window. A widget's page runs in a sandboxed frame of the home page
// (home.js), with no access to the window and no network: home.js puts the window's colors
// (tokens.css), widget.css and this file at the top of its <head>, and answers its messages.
// It gets `aex`:
//   aex.call(name, args)  one of its data calls, a Promise: built-in widgets call the widget APIs in
//                         home.go (widgetAPIs), e.g. aex.call('quota'); a plugin's widget its
//                         plugin's calls (plugin.Widget.Calls). Rejects with an Error.
//   aex.run(line, opts)   runs a tool on the Runs page, as if typed on the command line; a plugin's
//                         widget only its plugin's tools. opts {home: true}: back to Home once it
//                         finishes without failing, if Runs is still shown then
//   aex.remind(opts)      shows a reminder on top of every window on every screen, with a chime,
//                         until dismissed: opts {title (optional), message, urgent (chimes
//                         twice, again every 30 s for 10 minutes)}, a Promise
//   aex.onRefresh(fn)     fn runs when the data may have changed (after a run), on auto refresh (as
//                         often as the widget asks: WidgetInfo.Refresh) and on aex.refresh()
//   aex.settings          this placement's own settings (an object, {} at first): each time the
//                         widget is added it starts with none, e.g. which calendar it shows
//   aex.saveSettings(obj) replaces them (a JSON object, 8 KB at most), a Promise; kept in home.json
//   aex.onSettings(fn)    fn runs when the settings were changed by the widget's other page (its
//                         popup, or the cell while the popup is open); aex.settings has them already
//   aex.openPopup(opts)   opens this widget's page again in a popup over the window, for what does not
//                         fit in its cell (e.g. its settings): opts {title, width (px), data (JSON)}.
//                         A Promise of what the popup closes with (aex.closePopup(value)), undefined
//                         when it is closed otherwise (×, Esc, a click outside, leaving Home). One
//                         popup at a time; it rejects while another is open.
//   aex.popup             in the popup: {data} (opts.data); null in the cell
//   aex.closePopup(value) in the popup: closes it, openPopup resolving with value (JSON)
// The theme follows the window's.
const aex = (() => {
  // home.js puts aexSettings (this placement's settings) before this file.
  let settings = typeof aexSettings === 'object' && aexSettings ? aexSettings : {};
  // and aexPopup: {data} in a popup, null in the cell.
  const popup = typeof aexPopup === 'object' && aexPopup ? Object.freeze(aexPopup) : null;
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
      const root = document.documentElement;
      root.dataset.theme = m.theme;
      // The theme's colors (variables of tokens.css) replace the ones set before.
      for (const k of [...root.style]) if (k.startsWith('--')) root.style.removeProperty(k);
      for (const [k, v] of Object.entries(m.vars || {})) if (k.startsWith('--')) root.style.setProperty(k, String(v));
    } else if (m.op === 'refresh') {
      dispatchEvent(new Event('aex:refresh'));
    } else if (m.op === 'settings') {
      settings = m.settings && typeof m.settings === 'object' ? m.settings : {};
      dispatchEvent(new Event('aex:settings'));
    }
  });
  const request = (op, fields) => new Promise((resolve, reject) => {
    const id = ++seq;
    pending.set(id, { resolve, reject });
    parent.postMessage({ aex: 1, op, id, ...fields }, '*');
  });
  const closePopup = value => parent.postMessage({ aex: 1, op: 'close', id: ++seq, value: JSON.parse(JSON.stringify(value ?? null)) ?? undefined }, '*');
  if (popup) {
    // The popup is as tall as the page (up to the window's height): it tells the window its height.
    document.documentElement.classList.add('aex-popup');
    const size = () => parent.postMessage({ aex: 1, op: 'size', id: ++seq, height: Math.ceil(document.body?.scrollHeight || 0) }, '*');
    addEventListener('DOMContentLoaded', () => {
      new ResizeObserver(size).observe(document.body);
      size();
    });
    // Esc closes it, unless the page used the key itself (preventDefault).
    addEventListener('keydown', e => {
      if (e.key === 'Escape') setTimeout(() => { if (!e.defaultPrevented) closePopup(); });
    });
  }
  return Object.freeze({
    call: (name, args = {}) => request('call', { name, args }),
    run: (line, opts = {}) => request('run', { line, home: !!opts?.home }),
    remind: (opts = {}) => request('remind', { title: String(opts?.title ?? ''), message: String(opts?.message ?? ''), urgent: !!opts?.urgent }),
    onRefresh: fn => addEventListener('aex:refresh', () => fn()),
    refresh: () => dispatchEvent(new Event('aex:refresh')),
    get settings() { return structuredClone(settings); },
    onSettings: fn => addEventListener('aex:settings', () => fn()),
    popup,
    openPopup: (opts = {}) => request('popup', { opts: JSON.parse(JSON.stringify(opts ?? {})) }),
    closePopup: value => { if (popup) closePopup(value); },
    saveSettings: async obj => {
      const next = JSON.parse(JSON.stringify(obj ?? {}));
      await request('settings', { settings: next });
      settings = next;
    },
  });
})();
