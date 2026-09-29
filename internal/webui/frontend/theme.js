// The window's theme: system (the default), light or dark, kept in the webview's storage. Loaded
// in <head> so the page is drawn in its theme from the start. style.css keys on data-theme.
const theme = (() => {
  const key = 'theme';
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  const get = () => {
    try {
      const v = localStorage.getItem(key);
      return v === 'light' || v === 'dark' ? v : 'system';
    } catch { return 'system'; }
  };
  const apply = () => {
    const t = get();
    document.documentElement.dataset.theme = t === 'system' ? (system.matches ? 'dark' : 'light') : t;
  };
  const set = (t) => {
    try {
      if (t === 'system') localStorage.removeItem(key);
      else localStorage.setItem(key, t);
    } catch {}
    apply();
  };
  system.addEventListener('change', apply);
  apply();
  return { get, set };
})();
