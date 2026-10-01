// The window's theme: its mode (system, the default, light or dark) and its colors (a theme from
// themes.go: the default one, a built-in one or a custom one), both kept in the webview's storage.
// Loaded in <head> so the page is drawn in its theme from the start: the colors picked are kept
// with the choice and refreshed from the backend once it is there (theme.refresh).
// style.css keys on data-theme (light or dark); the colors are the variables of tokens.css, set on
// <html>. Each change fires "aex:theme" on window; the widgets get it too (home.js).
const theme = (() => {
  const key = 'theme', paletteKey = 'palette';
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  const root = document.documentElement;
  const get = () => {
    try {
      const v = localStorage.getItem(key);
      return v === 'light' || v === 'dark' ? v : 'system';
    } catch { return 'system'; }
  };
  // The theme picked: {id, name, light, dark}; the default one has no colors.
  let palette = { id: 'default' };
  try {
    const p = JSON.parse(localStorage.getItem(paletteKey) || 'null');
    if (p && typeof p.id === 'string') palette = p;
  } catch {}
  let applied = [];
  // scheme is light or dark: the mode picked, unless the theme has only one of them.
  const scheme = () => {
    if (palette.light && !palette.dark) return 'light';
    if (palette.dark && !palette.light) return 'dark';
    const t = get();
    return t === 'system' ? (system.matches ? 'dark' : 'light') : t;
  };
  // vars are the variables the theme sets now, by name with "--".
  const vars = () => {
    const out = {};
    for (const [k, v] of Object.entries(palette[scheme()] || {})) out['--' + k] = v;
    return out;
  };
  const apply = () => {
    root.dataset.theme = scheme();
    applied.forEach(k => root.style.removeProperty(k));
    const v = vars();
    applied = Object.keys(v);
    for (const k of applied) root.style.setProperty(k, v[k]);
    window.dispatchEvent(new Event('aex:theme'));
  };
  const set = (t) => {
    try {
      if (t === 'system') localStorage.removeItem(key);
      else localStorage.setItem(key, t);
    } catch {}
    apply();
  };
  // setPalette picks a theme (from api().Themes()).
  const setPalette = (p) => {
    palette = p && p.id !== 'default' ? { id: p.id, name: p.name, light: p.light, dark: p.dark } : { id: 'default' };
    try {
      if (palette.id === 'default') localStorage.removeItem(paletteKey);
      else localStorage.setItem(paletteKey, JSON.stringify(palette));
    } catch {}
    apply();
  };
  // refresh takes the theme picked again from the list (a custom theme's file may have changed),
  // going back to the default one when it is gone. Gives the list.
  const refresh = async () => {
    const list = await window.go.webui.App.Themes();
    if (palette.id !== 'default') setPalette(list.themes.find(t => t.id === palette.id));
    return list;
  };
  system.addEventListener('change', apply);
  apply();
  return { get, set, palette: () => palette, setPalette, refresh, vars, scheme };
})();
