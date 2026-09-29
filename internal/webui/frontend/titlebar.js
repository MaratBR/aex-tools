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
})();
