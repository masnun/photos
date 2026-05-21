const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

const escapeHtml = (s) => String(s ?? '').replace(/[&<>"']/g, c => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
}[c]));

let rows = [];

async function api(path, opts = {}) {
  const r = await fetch(path, opts);
  if (!r.ok) {
    const text = await r.text().catch(() => '');
    throw new Error(`${r.status}: ${text}`);
  }
  return r.status === 204 ? null : r.json();
}

async function load() {
  rows = await api('/api/rows');
  refreshGenreFilter();
  render();
}

function refreshGenreFilter() {
  const set = new Set();
  rows.forEach(r => r.genres.forEach(g => set.add(g)));
  const sel = $('#genre-filter');
  const current = sel.value;
  const genres = [...set].sort();
  sel.innerHTML = '<option value="">all genres</option>' +
    genres.map(g => `<option value="${escapeHtml(g)}">${escapeHtml(g)}</option>`).join('');
  if (genres.includes(current)) sel.value = current;
}

function filtered() {
  const q = $('#search').value.toLowerCase().trim();
  const g = $('#genre-filter').value;
  const untagged = $('#untagged-only').checked;
  return rows.filter(r => {
    if (untagged && r.genres.length > 0) return false;
    if (g && !r.genres.includes(g)) return false;
    if (q && !r.path.toLowerCase().includes(q) && !r.caption.toLowerCase().includes(q)) return false;
    return true;
  });
}

function render() {
  const list = filtered();
  $('#stat-total').textContent = rows.length;
  $('#stat-untagged').textContent = rows.filter(r => r.genres.length === 0).length;
  $('#stat-visible').textContent = list.length;

  const grid = $('#grid');
  grid.innerHTML = list.map(card).join('');

  $$('.card').forEach(el => {
    const idx = +el.dataset.idx;
    $('.save', el).addEventListener('click', () => save(idx, el));
    $('.skip', el).addEventListener('click', () => skip(idx));
    $('.thumb', el).addEventListener('click', () => {
      window.open('/full/' + encodeURI(rows[idx].path), '_blank');
    });
  });
}

function card(r) {
  return `
    <article class="card" data-idx="${r.idx}">
      <img class="thumb" src="/thumb/${encodeURI(r.path)}" loading="lazy" alt="">
      <div class="fields">
        <div class="path" title="${escapeHtml(r.path)}">${escapeHtml(r.path)}</div>
        <label>genres (comma-sep)
          <input class="genres" value="${escapeHtml(r.genres.join(', '))}">
        </label>
        <label>caption
          <textarea class="caption" rows="2">${escapeHtml(r.caption)}</textarea>
        </label>
        <label>collection
          <input class="collection" value="${escapeHtml(r.collection)}">
        </label>
        <div class="actions">
          <button class="save">Save</button>
          <button class="skip" title="remove row from TSV — won't be ingested">Skip</button>
          <span class="status"></span>
        </div>
      </div>
    </article>`;
}

async function save(idx, el) {
  const genres = $('.genres', el).value.split(',').map(s => s.trim()).filter(Boolean);
  const caption = $('.caption', el).value;
  const collection = $('.collection', el).value;
  const status = $('.status', el);
  status.textContent = '…';
  try {
    const updated = await api(`/api/rows/${idx}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ genres, caption, collection })
    });
    const i = rows.findIndex(r => r.idx === idx);
    if (i >= 0) rows[i] = updated;
    status.textContent = 'saved';
    el.classList.add('dirty-saved');
    refreshGenreFilter();
    setTimeout(() => { status.textContent = ''; el.classList.remove('dirty-saved'); }, 1500);
  } catch (err) {
    status.textContent = 'error: ' + err.message;
  }
}

async function skip(idx) {
  if (!confirm('Remove this row from classifications.tsv? (will not be ingested)')) return;
  try {
    await api(`/api/rows/${idx}`, { method: 'DELETE' });
    await load();
  } catch (err) {
    alert('skip failed: ' + err.message);
  }
}

$('#search').addEventListener('input', debounce(render, 200));
$('#genre-filter').addEventListener('change', render);
$('#untagged-only').addEventListener('change', render);
$('#reload').addEventListener('click', async () => {
  await api('/api/reload', { method: 'POST' });
  await load();
});

function debounce(fn, ms) {
  let t;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), ms);
  };
}

load().catch(err => alert('load failed: ' + err.message));
