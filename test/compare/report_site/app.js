// Renders data/report.json (written by cmd/fitz-report) and, when present,
// build.json (stamped by the publish workflow). All report text is inserted
// with textContent, never as HTML.

const STATUS_LABELS = {
  pass: 'Pass',
  typed_unsupported: 'Typed unsupported',
  mismatch: 'Mismatch',
  error: 'Error',
  unexpected: 'Unexpected',
};

let allResults = [];
let activeFilter = 'all';

function el(tag, text, className) {
  const node = document.createElement(tag);
  if (text !== undefined && text !== null) node.textContent = text;
  if (className) node.className = className;
  return node;
}

function summaryCard(label, value, className) {
  const card = el('div', null, `card ${className || ''}`.trim());
  card.append(el('div', String(value), 'card-value'), el('div', label, 'card-label'));
  return card;
}

function renderSummary(report) {
  const s = report.summary;
  const cards = document.getElementById('summary-cards');
  cards.replaceChildren(
    summaryCard('Cases', s.total),
    summaryCard('Pass', s.pass, 'ok'),
    summaryCard('Typed unsupported', s.typedUnsupported, 'muted'),
    summaryCard('Mismatch', s.mismatch, s.mismatch ? 'bad' : ''),
    summaryCard('Error', s.error, s.error ? 'bad' : ''),
    summaryCard('Unexpected', s.unexpected, s.unexpected ? 'bad' : ''),
    summaryCard(s.green ? 'Gate green' : 'Gate failing', s.green ? 'Yes' : 'No', s.green ? 'ok' : 'bad'),
  );
  const generated = new Date(report.generated);
  document.getElementById('meta').textContent =
    `Reference MuPDF ${report.mupdf.version} (${report.mupdf.commit.slice(0, 12)}). ` +
    `Generated ${Number.isNaN(generated.getTime()) ? report.generated : generated.toUTCString()}.`;
}

function isFailing(r) {
  return r.status !== 'pass' && r.status !== 'typed_unsupported';
}

function matchesFilter(r) {
  if (activeFilter === 'all') return true;
  if (activeFilter === 'failing') return isFailing(r);
  return r.status === activeFilter;
}

function renderResults() {
  const rows = allResults.filter(matchesFilter).map((r) => {
    const tr = el('tr', null, isFailing(r) ? 'row-failing' : '');
    tr.append(
      el('td', r.asset),
      el('td', r.method),
      el('td', r.expect),
      statusCell(r.status),
      detailCell(r),
    );
    return tr;
  });
  const body = document.getElementById('results');
  if (rows.length === 0) {
    const tr = el('tr');
    const td = el('td', 'No results match this filter.', 'empty');
    td.colSpan = 5;
    tr.append(td);
    body.replaceChildren(tr);
  } else {
    body.replaceChildren(...rows);
  }
}

function statusCell(status) {
  const td = el('td');
  td.append(el('span', STATUS_LABELS[status] || status, `status status-${status}`));
  return td;
}

function detailCell(r) {
  const td = el('td');
  if (r.detail) td.append(el('div', r.detail, 'detail'));
  if (r.got !== undefined || r.want !== undefined) {
    const details = el('details');
    details.append(el('summary', 'Got and want'));
    details.append(el('pre', `got:  ${JSON.stringify(r.got, null, 2)}\nwant: ${JSON.stringify(r.want, null, 2)}`));
    td.append(details);
  }
  return td;
}

function wireFilters() {
  document.querySelectorAll('.filter button').forEach((button) => {
    button.addEventListener('click', () => {
      activeFilter = button.dataset.filter;
      document.querySelectorAll('.filter button').forEach((b) => {
        const on = b === button;
        b.classList.toggle('is-active', on);
        b.setAttribute('aria-pressed', String(on));
      });
      renderResults();
    });
  });
}

async function loadBuild() {
  try {
    const res = await fetch('build.json', { cache: 'no-store' });
    if (!res.ok) return;
    const b = await res.json();
    document.getElementById('build-meta').textContent =
      `Commit ${String(b.commit).slice(0, 12)} on ${b.ref}. Published ${b.generated}. ` +
      (b.run ? `Workflow run: ${b.run}` : '');
  } catch {
    // build.json is optional when the site is opened locally.
  }
}

async function main() {
  wireFilters();
  loadBuild();
  try {
    const res = await fetch('data/report.json', { cache: 'no-store' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const report = await res.json();
    allResults = report.results;
    renderSummary(report);
    renderResults();
  } catch (err) {
    document.getElementById('meta').textContent = `Could not load data/report.json: ${err.message}`;
  }
}

main();
