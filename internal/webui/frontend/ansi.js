// Terminal output to DOM: SGR styles (bold, dim, italic, underline, 16/256/true colors), "\n" and a
// lone "\r" (rewrite the current line). Other escape sequences are dropped.
class Terminal {
  constructor(el) {
    this.el = el;
    this.pending = '';
    this.style = {};
    this.clear();
  }

  clear() {
    this.el.textContent = '';
    this.line = this.newLine();
  }

  newLine() {
    const div = document.createElement('div');
    div.className = 'line';
    this.el.appendChild(div);
    return div;
  }

  write(text) {
    text = this.pending + text;
    this.pending = '';
    // Keep an escape sequence cut off at the end for the next write.
    const esc = text.lastIndexOf('\x1b');
    if (esc >= 0 && !/^\x1b(\[[0-9;?]*[A-Za-z]|\][^\x07\x1b]*(\x07|\x1b\\)|[^[\]])/.test(text.slice(esc))) {
      this.pending = text.slice(esc);
      text = text.slice(0, esc);
    }
    const re = /\x1b\[([0-9;?]*)([A-Za-z])|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[^[\]]|\r\n|\r|\n/g;
    let last = 0, m;
    while ((m = re.exec(text))) {
      this.put(text.slice(last, m.index));
      last = re.lastIndex;
      const s = m[0];
      if (s === '\n' || s === '\r\n') this.line = this.newLine();
      else if (s === '\r') this.line.textContent = '';
      else if (m[2] === 'm') this.sgr(m[1]);
    }
    this.put(text.slice(last));
    this.el.scrollTop = this.el.scrollHeight;
  }

  put(s) {
    if (!s) return;
    const st = this.style;
    if (!st.fg && !st.bg && !st.bold && !st.dim && !st.italic && !st.underline) {
      this.line.appendChild(document.createTextNode(s));
      return;
    }
    const span = document.createElement('span');
    span.textContent = s;
    const cls = [];
    if (st.bold) cls.push('b');
    if (st.dim) cls.push('d');
    if (st.italic) cls.push('i');
    if (st.underline) cls.push('u');
    if (typeof st.fg === 'number') cls.push('f' + st.fg); else if (st.fg) span.style.color = st.fg;
    if (typeof st.bg === 'number') cls.push('g' + st.bg); else if (st.bg) span.style.background = st.bg;
    span.className = cls.join(' ');
    this.line.appendChild(span);
  }

  sgr(params) {
    const p = params === '' ? [0] : params.split(';').map(Number);
    const st = this.style;
    for (let i = 0; i < p.length; i++) {
      const n = p[i];
      if (n === 0) for (const k of Object.keys(st)) delete st[k];
      else if (n === 1) st.bold = true;
      else if (n === 2) st.dim = true;
      else if (n === 3) st.italic = true;
      else if (n === 4) st.underline = true;
      else if (n === 22) st.bold = st.dim = false;
      else if (n === 23) st.italic = false;
      else if (n === 24) st.underline = false;
      else if (n >= 30 && n <= 37) st.fg = n - 30;
      else if (n >= 90 && n <= 97) st.fg = n - 90 + 8;
      else if (n === 39) st.fg = undefined;
      else if (n >= 40 && n <= 47) st.bg = n - 40;
      else if (n >= 100 && n <= 107) st.bg = n - 100 + 8;
      else if (n === 49) st.bg = undefined;
      else if (n === 38 || n === 48) {
        let c;
        if (p[i + 1] === 5) { c = color256(p[i + 2]); i += 2; }
        else if (p[i + 1] === 2) { c = `rgb(${p[i + 2]},${p[i + 3]},${p[i + 4]})`; i += 4; }
        if (n === 38) st.fg = c; else st.bg = c;
      }
    }
  }
}

// 256-color index: 0-15 stay theme colors (classes), the rest are fixed.
function color256(n) {
  if (n < 16) return n;
  if (n >= 232) { const v = 8 + (n - 232) * 10; return `rgb(${v},${v},${v})`; }
  n -= 16;
  const c = v => (v ? 55 + v * 40 : 0);
  return `rgb(${c(Math.floor(n / 36))},${c(Math.floor(n / 6) % 6)},${c(n % 6)})`;
}

// ansiToFragment renders a short styled string (a label, a header line) on its own.
function ansiToFragment(text) {
  const holder = document.createElement('span');
  const t = new Terminal(holder);
  t.write(text);
  const frag = document.createDocumentFragment();
  holder.querySelectorAll('.line').forEach((line, i) => {
    if (i) frag.appendChild(document.createElement('br'));
    while (line.firstChild) frag.appendChild(line.firstChild);
  });
  return frag;
}
