// The window is frameless (webui.go): #titlebar and the sidebar's brand row drag it, and the
// buttons in #titlebar stand in for the ones Windows would draw.
(() => {
  const rt = window.runtime;
  const $ = (id) => document.getElementById(id);

  // syncMaximised swaps the maximize button for restore while the window fills the screen.
  const syncMaximised = async () => {
    const max = await rt.WindowIsMaximised();
    document.body.classList.toggle('maximised', max);
    const b = $('win-max');
    b.title = b.ariaLabel = max ? 'Restore' : 'Maximize';
    // Wails draws resize cursors at the window's edges itself and turns that off for fullscreen
    // only: a maximised window can't be resized, so its edges are left alone too.
    const flags = window.wails.flags;
    flags.enableResize = !max;
    if (max && flags.resizeEdge) {
      flags.resizeEdge = undefined;
      document.documentElement.style.cursor = flags.defaultCursor;
    }
  };

  $('win-min').onclick = () => rt.WindowMinimise();
  $('win-max').onclick = () => rt.WindowToggleMaximise();
  $('win-close').onclick = () => rt.Quit();

  // Wails starts a drag on a single click only, so a double click is left to toggle maximize.
  const onDoubleClick = (e) => {
    if (e.target.closest('button')) return;
    rt.WindowToggleMaximise();
  };
  $('titlebar').addEventListener('dblclick', onDoubleClick);
  document.querySelector('.brand').addEventListener('dblclick', onDoubleClick);

  window.addEventListener('resize', syncMaximised);
  syncMaximised();

  // The title bar is see-through until the pointer comes near: it fades in over the top farPx of
  // the window, fully shown within nearPx of the top (style.css: --near).
  const nearPx = 12, farPx = 160;
  const bar = $('titlebar');
  let y = Infinity, frame = 0;
  const show = () => {
    frame = 0;
    const near = Math.min(1, Math.max(0, (farPx - y) / (farPx - nearPx)));
    bar.style.setProperty('--near', near.toFixed(3));
  };
  const track = (at) => {
    y = at;
    if (!frame) frame = requestAnimationFrame(show);
  };
  document.addEventListener('mousemove', (e) => track(e.clientY));
  // Left the window (or went into a widget's frame, which keeps its moves): faint again.
  document.documentElement.addEventListener('mouseleave', () => track(Infinity));
  show();
})();
