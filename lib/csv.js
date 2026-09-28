function escapeCell(value) {
  const s = value == null ? '' : String(value);
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function toCsv(columns, rows) {
  const lines = [columns.map((c) => escapeCell(c.header)).join(',')];
  for (const row of rows) lines.push(columns.map((c) => escapeCell(c.value(row))).join(','));
  return lines.join('\r\n') + '\r\n';
}

// Parses CSV with a header row into objects keyed by header. Handles quoted cells.
export function parseCsv(text) {
  const records = [];
  let record = [];
  let cell = '';
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (quoted) {
      if (ch === '"' && text[i + 1] === '"') (cell += '"'), i++;
      else if (ch === '"') quoted = false;
      else cell += ch;
    } else if (ch === '"') quoted = true;
    else if (ch === ',') record.push(cell), (cell = '');
    else if (ch === '\n' || ch === '\r') {
      if (ch === '\r' && text[i + 1] === '\n') i++;
      record.push(cell);
      records.push(record);
      record = [];
      cell = '';
    } else cell += ch;
  }
  if (cell || record.length) record.push(cell), records.push(record);

  const [header, ...rows] = records.filter((r) => r.some((c) => c !== ''));
  if (!header) return [];
  return rows.map((r) => Object.fromEntries(header.map((h, i) => [h.trim(), r[i] ?? ''])));
}
