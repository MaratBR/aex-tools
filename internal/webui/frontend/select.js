// Drop-down lists in the window's colors. Every <select> on the page (the window's and each
// widget's: home.js puts this file in their pages) is shown as a button that opens a list like Add
// widget's, since a native one opens in the system's colors, not the theme's. The <select> stays,
// hidden, holding the options and the value: code sets and reads select.value and listens for
// change as before; picking an option sets it and fires input and change. A long list has a search
// field. An option's title shows under it.
(() => {
  const css = `
.xs-native { display: none !important; }
.xs-btn { display: inline-flex; align-items: center; gap: 6px; min-width: 0; text-align: left; cursor: pointer; }
.xs-btn:disabled { cursor: default; opacity: .55; }
.xs-btn .xs-text { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.xs-btn svg, .xs-item svg { flex: none; width: 12px; height: 12px; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
.xs-btn svg { color: var(--muted); transition: transform .15s; }
.xs-btn[aria-expanded="true"] svg { transform: rotate(180deg); }
.xs-pop {
  position: fixed; z-index: 1000; box-sizing: border-box;
  display: flex; flex-direction: column; gap: 4px; padding: 6px;
  background: var(--surface); color: var(--ink); border-radius: 12px; box-shadow: var(--pop-shadow);
  font: 13px/1.4 var(--sans); letter-spacing: -.005em; user-select: none;
  animation: xs-in .12s ease-out;
}
.xs-pop:focus { outline: none; box-shadow: var(--pop-shadow); }
@keyframes xs-in { from { opacity: 0; transform: translateY(-3px); } }
.xs-pop.up { animation-name: xs-in-up; }
@keyframes xs-in-up { from { opacity: 0; transform: translateY(3px); } }
.xs-search {
  flex: none; box-sizing: border-box; width: 100%; padding: 5px 9px; border: 0; border-radius: 8px;
  font: inherit; color: var(--ink); background: var(--fill); outline: none;
}
.xs-search:focus { box-shadow: var(--ring); }
.xs-search::placeholder { color: var(--faint); }
.xs-list { min-height: 0; overflow-y: auto; display: flex; flex-direction: column; gap: 1px; scrollbar-width: thin; scrollbar-color: var(--faint) transparent; }
.xs-group { padding: 6px 10px 2px; color: var(--muted); font-size: 11px; font-weight: 600; }
.xs-item {
  display: flex; align-items: center; gap: 8px; width: 100%; box-sizing: border-box;
  padding: 6px 10px; border: 0; border-radius: 8px; background: none; color: inherit;
  font: inherit; text-align: left; cursor: pointer;
}
.xs-item.active { background: var(--hover); }
.xs-item[aria-disabled="true"] { opacity: .45; cursor: default; }
.xs-item .xs-body { flex: 1; min-width: 0; }
.xs-item .xs-label { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.xs-item .xs-sub { display: block; color: var(--muted); font-size: 11px; line-height: 1.35; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.xs-item[aria-selected="true"] { color: var(--accent); font-weight: 600; }
.xs-item[aria-selected="true"] .xs-sub { font-weight: 400; }
.xs-item svg { visibility: hidden; }
.xs-item[aria-selected="true"] svg { visibility: visible; }
.xs-empty { padding: 8px 10px; color: var(--muted); }
@media (prefers-reduced-motion: reduce) { .xs-pop { animation: none; } }
`;
  const chevron = '<svg viewBox="0 0 12 12" aria-hidden="true"><path d="M3 4.5l3 3 3-3"/></svg>';
  const check = '<svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2.5 6.5l2.5 2.5 4.5-5"/></svg>';
  const searchFrom = 10; // options: a list this long gets a search field

  const proto = HTMLSelectElement.prototype;
  const valueProp = Object.getOwnPropertyDescriptor(proto, 'value');
  const indexProp = Object.getOwnPropertyDescriptor(proto, 'selectedIndex');
  let open = null; // the list open now: {select, close}

  function enhance(select) {
    if (select.xsButton || select.multiple || select.size > 1) return;
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'xs-btn ' + select.className;
    btn.setAttribute('aria-haspopup', 'listbox');
    btn.setAttribute('aria-expanded', 'false');
    const text = document.createElement('span');
    text.className = 'xs-text';
    btn.append(text);
    btn.insertAdjacentHTML('beforeend', chevron);
    select.classList.add('xs-native');
    select.after(btn);
    select.xsButton = btn;

    const sync = () => {
      const o = select.options[indexProp.get.call(select)];
      text.textContent = o ? o.textContent : '';
      btn.title = select.title || (o ? o.title || o.textContent : '');
      btn.disabled = select.disabled;
      const label = select.getAttribute('aria-label') || select.ariaLabel;
      if (label) btn.setAttribute('aria-label', label);
      else btn.removeAttribute('aria-label');
    };
    // Setting the value or the index in code shows at once; so do new options.
    Object.defineProperty(select, 'value', { configurable: true, get: () => valueProp.get.call(select), set: v => { valueProp.set.call(select, v); sync(); } });
    Object.defineProperty(select, 'selectedIndex', { configurable: true, get: () => indexProp.get.call(select), set: v => { indexProp.set.call(select, v); sync(); } });
    new MutationObserver(sync).observe(select, { childList: true, subtree: true, attributes: true, characterData: true });
    select.focus = opts => btn.focus(opts);
    // A label's click goes to the select: open the list.
    select.addEventListener('click', () => { if (!btn.disabled) show(select, btn); });
    btn.addEventListener('click', () => (open?.select === select ? open.close() : show(select, btn)));
    btn.addEventListener('keydown', e => {
      if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key) || (e.altKey && e.key === 'ArrowDown')) {
        e.preventDefault();
        show(select, btn);
      }
    });
    sync();
  }

  function pick(select, i) {
    if (i === indexProp.get.call(select)) return;
    select.selectedIndex = i;
    select.dispatchEvent(new Event('input', { bubbles: true }));
    select.dispatchEvent(new Event('change', { bubbles: true }));
  }

  function show(select, btn) {
    open?.close();
    const pop = document.createElement('div');
    pop.className = 'xs-pop';
    pop.setAttribute('role', 'listbox');
    const options = [...select.options];
    let search = null;
    if (options.length >= searchFrom) {
      search = document.createElement('input');
      search.className = 'xs-search';
      search.type = 'text';
      search.placeholder = 'Search';
      search.spellcheck = false;
      search.autocomplete = 'off';
      pop.append(search);
    }
    const list = document.createElement('div');
    list.className = 'xs-list';
    pop.append(list);

    let items = [], active = -1;
    const setActive = (n, scroll = true) => {
      items[active]?.el.classList.remove('active');
      active = n;
      const it = items[active];
      if (!it) return;
      it.el.classList.add('active');
      // Scrolled by hand: scrollIntoView could scroll the page, which closes the list.
      if (scroll) {
        const top = it.el.offsetTop - list.offsetTop, bottom = top + it.el.offsetHeight;
        if (top < list.scrollTop) list.scrollTop = top;
        else if (bottom > list.scrollTop + list.clientHeight) list.scrollTop = bottom - list.clientHeight;
      }
    };
    const fill = () => {
      const words = (search?.value || '').toLowerCase().split(/\s+/).filter(Boolean);
      list.textContent = '';
      items = [];
      let group = null;
      options.forEach((o, i) => {
        if (o.hidden) return;
        const hay = (o.textContent + ' ' + o.value + ' ' + o.title).toLowerCase();
        if (!words.every(w => hay.includes(w))) return;
        const g = o.parentElement instanceof HTMLOptGroupElement ? o.parentElement : null;
        if (g && g !== group) {
          const h = document.createElement('div');
          h.className = 'xs-group';
          h.textContent = g.label;
          list.append(h);
        }
        group = g;
        const el = document.createElement('div');
        el.className = 'xs-item';
        el.setAttribute('role', 'option');
        el.setAttribute('aria-selected', String(i === indexProp.get.call(select)));
        const disabled = o.disabled || !!g?.disabled;
        if (disabled) el.setAttribute('aria-disabled', 'true');
        el.insertAdjacentHTML('afterbegin', check);
        const body = document.createElement('span');
        body.className = 'xs-body';
        const label = document.createElement('span');
        label.className = 'xs-label';
        label.textContent = o.textContent;
        body.append(label);
        if (o.title && o.title !== o.textContent) {
          const sub = document.createElement('span');
          sub.className = 'xs-sub';
          sub.textContent = o.title;
          body.append(sub);
        }
        el.append(body);
        const n = items.length;
        items.push({ el, i, disabled });
        el.addEventListener('pointermove', () => { if (active !== n) setActive(n, false); });
        el.addEventListener('click', () => { if (!disabled) { pick(select, i); close(); btn.focus(); } });
        list.append(el);
      });
      if (!items.length) {
        const none = document.createElement('div');
        none.className = 'xs-empty';
        none.textContent = 'Nothing matches';
        list.append(none);
      }
    };
    fill();

    // Below the button, or above when there is more room there; as wide as the button at least.
    const place = () => {
      const r = btn.getBoundingClientRect(), vw = document.documentElement.clientWidth, vh = document.documentElement.clientHeight;
      const below = vh - r.bottom - 6, above = r.top - 6;
      const up = below < 220 && above > below;
      const room = Math.max(80, (up ? above : below) - 6);
      const width = Math.min(Math.max(r.width, 200), vw - 12);
      pop.style.width = width + 'px';
      pop.style.left = Math.max(6, Math.min(r.left, vw - width - 6)) + 'px';
      pop.style.maxHeight = Math.min(room, 360) + 'px';
      pop.classList.toggle('up', up);
      if (up) { pop.style.top = ''; pop.style.bottom = (vh - r.top + 4) + 'px'; }
      else { pop.style.bottom = ''; pop.style.top = (r.bottom + 4) + 'px'; }
    };
    document.body.append(pop);
    place();
    btn.setAttribute('aria-expanded', 'true');
    const current = items.findIndex(it => it.i === indexProp.get.call(select));
    setActive(current >= 0 ? current : 0);

    let typed = '', typedAt = 0;
    const onKey = e => {
      const move = d => {
        if (!items.length) return;
        let n = active;
        for (let k = 0; k < items.length; k++) {
          n = (n + d + items.length) % items.length;
          if (!items[n].disabled) break;
        }
        setActive(n);
      };
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); btn.focus(); }
      else if (e.key === 'ArrowDown') { e.preventDefault(); move(1); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); move(-1); }
      else if (e.key === 'Home' && !search) { e.preventDefault(); setActive(0); }
      else if (e.key === 'End' && !search) { e.preventDefault(); setActive(items.length - 1); }
      else if (e.key === 'Enter' || (e.key === ' ' && !search)) {
        e.preventDefault();
        const it = items[active];
        if (it && !it.disabled) pick(select, it.i);
        close();
        btn.focus();
      } else if (e.key === 'Tab') close();
      else if (!search && e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
        // Typing jumps to the option that starts with it.
        typed = (Date.now() - typedAt > 700 ? '' : typed) + e.key.toLowerCase();
        typedAt = Date.now();
        const n = items.findIndex(it => options[it.i].textContent.toLowerCase().startsWith(typed));
        if (n >= 0) setActive(n);
      }
    };
    const onDown = e => { if (!pop.contains(e.target) && e.target !== btn && !btn.contains(e.target)) close(); };
    const onScroll = e => { if (!pop.contains(e.target)) close(); };
    function close() {
      if (open?.pop !== pop) return;
      open = null;
      pop.remove();
      btn.setAttribute('aria-expanded', 'false');
      document.removeEventListener('keydown', onKey, true);
      document.removeEventListener('pointerdown', onDown, true);
      document.removeEventListener('scroll', onScroll, true);
      removeEventListener('resize', place);
      removeEventListener('blur', close);
    }
    open = { select, pop, close };
    document.addEventListener('keydown', onKey, true);
    document.addEventListener('pointerdown', onDown, true);
    document.addEventListener('scroll', onScroll, true);
    addEventListener('resize', place); // a widget's popup grows and shrinks with its page
    addEventListener('blur', close); // a click elsewhere in the window, outside this frame
    if (search) {
      search.addEventListener('input', () => { fill(); setActive(items.findIndex(it => !it.disabled)); });
      search.focus({ preventScroll: true });
    } else {
      pop.tabIndex = -1;
      pop.focus({ preventScroll: true });
    }
  }

  function scan(root) {
    if (root instanceof HTMLSelectElement) return enhance(root);
    root.querySelectorAll?.('select').forEach(enhance);
  }
  const start = () => {
    const style = document.createElement('style');
    style.textContent = css;
    document.head.append(style);
    scan(document);
    new MutationObserver(records => {
      for (const r of records) r.addedNodes.forEach(n => n.nodeType === 1 && scan(n));
    }).observe(document.documentElement, { childList: true, subtree: true });
  };
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
