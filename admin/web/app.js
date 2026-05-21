const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

const api = async (path, opts = {}) => {
  const r = await fetch(`/api${path}`, opts);
  if (!r.ok) {
    let msg = `${r.status}`;
    try { const j = await r.json(); if (j.error) msg = j.error; } catch {}
    throw new Error(msg);
  }
  return r.status === 204 ? null : r.json();
};

const escapeHtml = (s) => String(s ?? '').replace(/[&<>"']/g, c => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
}[c]));

let photos = [], genres = [], collections = [];
let genreFilter = 'all';
let collectionFilter = 'all';
let dateFrom = '';
let dateTo = '';

async function refresh() {
  [photos, genres, collections] = await Promise.all([
    api('/photos'), api('/genres'), api('/collections')
  ]);
  renderFilter();
  renderPhotos();
  renderGenres();
  renderCollections();
}

function matchOne(filter, list) {
  if (filter === 'all') return true;
  if (filter === 'untagged') return !list || list.length === 0;
  return list && list.includes(filter);
}

function photoDate(p) {
  return p.taken_at || p.uploaded_at || '';
}

function matchesDateRange(p) {
  if (!dateFrom && !dateTo) return true;
  const d = photoDate(p);
  if (!d) return false;
  const day = d.slice(0, 10);
  if (dateFrom && day < dateFrom) return false;
  if (dateTo && day > dateTo) return false;
  return true;
}

function photoMatchesFilter(p) {
  return matchOne(genreFilter, p.genres)
    && matchOne(collectionFilter, p.collections)
    && matchesDateRange(p);
}

function renderFilterRow(box, label, items, current, onPick, listKey) {
  const counts = { all: photos.length, untagged: 0 };
  items.forEach(it => counts[it.slug] = 0);
  photos.forEach(p => {
    const list = p[listKey] || [];
    if (list.length === 0) counts.untagged++;
    list.forEach(s => { if (s in counts) counts[s]++; });
  });
  const chip = (key, lbl) => `
    <button type="button" class="chip${current === key ? ' active' : ''}" data-filter="${escapeHtml(key)}">
      ${escapeHtml(lbl)} <span class="count">${counts[key] ?? 0}</span>
    </button>`;
  const row = document.createElement('div');
  row.className = 'filter-row';
  row.innerHTML = `<span class="filter-label">${escapeHtml(label)}</span>` + [
    chip('all', 'All'),
    ...items.map(it => chip(it.slug, it.name)),
    chip('untagged', 'Untagged'),
  ].join('');
  row.querySelectorAll('.chip').forEach(btn => {
    btn.addEventListener('click', () => {
      onPick(btn.dataset.filter);
      renderFilter();
      renderPhotos();
    });
  });
  box.appendChild(row);
}

function renderFilter() {
  const box = $('#photo-filter');
  if (!box) return;
  box.innerHTML = '';
  renderFilterRow(box, 'Genre', genres, genreFilter, v => genreFilter = v, 'genres');
  renderFilterRow(box, 'Collection', collections, collectionFilter, v => collectionFilter = v, 'collections');
  renderDateRow(box);
}

function renderDateRow(box) {
  const row = document.createElement('div');
  row.className = 'filter-row';
  row.innerHTML = `
    <span class="filter-label">Date</span>
    <input type="date" id="date-from" value="${escapeHtml(dateFrom)}">
    <span class="filter-sep">to</span>
    <input type="date" id="date-to" value="${escapeHtml(dateTo)}">
    <button type="button" id="date-clear" class="chip">Clear</button>`;
  box.appendChild(row);
  row.querySelector('#date-from').addEventListener('change', e => {
    dateFrom = e.target.value;
    renderPhotos();
  });
  row.querySelector('#date-to').addEventListener('change', e => {
    dateTo = e.target.value;
    renderPhotos();
  });
  row.querySelector('#date-clear').addEventListener('click', () => {
    dateFrom = '';
    dateTo = '';
    renderFilter();
    renderPhotos();
  });
}

function renderPhotos() {
  const grid = $('#photo-grid');
  grid.innerHTML = '';
  [...photos].reverse().filter(photoMatchesFilter).forEach(p => {
    const el = document.createElement('div');
    el.className = 'photo-card';
    const tags = [...p.genres, ...p.collections].map(t => `<span>${escapeHtml(t)}</span>`).join('');
    el.innerHTML = `
      <img src="${escapeHtml(p.urls.thumb)}" alt="">
      <div class="meta">
        <div class="tags">${tags || '<em>untagged</em>'}</div>
      </div>`;
    el.addEventListener('click', () => openEdit(p));
    grid.appendChild(el);
  });
}

function renderGenres() {
  const ul = $('#genre-list');
  ul.innerHTML = '';
  genres.forEach(g => {
    const li = document.createElement('li');
    li.innerHTML = `<strong>${escapeHtml(g.name)}</strong> <code>${escapeHtml(g.slug)}</code>
      <span>${escapeHtml(g.description || '')}</span>
      <button class="danger">Delete</button>`;
    li.querySelector('button').addEventListener('click', async () => {
      if (!confirm(`Delete genre "${g.slug}"? Photos referencing it will keep the dangling slug.`)) return;
      await api(`/genres/${encodeURIComponent(g.slug)}`, { method: 'DELETE' });
      refresh();
    });
    ul.appendChild(li);
  });
}

function renderCollections() {
  const ul = $('#collection-list');
  ul.innerHTML = '';
  collections.forEach(c => {
    const li = document.createElement('li');
    li.innerHTML = `<strong>${escapeHtml(c.name)}</strong> <code>${escapeHtml(c.slug)}</code>
      <span>${escapeHtml(c.description || '')}</span>
      <button class="danger">Delete</button>`;
    li.querySelector('button').addEventListener('click', async () => {
      if (!confirm(`Delete collection "${c.slug}"? Photos referencing it will keep the dangling slug.`)) return;
      await api(`/collections/${encodeURIComponent(c.slug)}`, { method: 'DELETE' });
      refresh();
    });
    ul.appendChild(li);
  });
}

function openEdit(p) {
  const dlg = $('#edit-photo');
  $('[name=id]', dlg).value = p.id;
  $('[name=caption]', dlg).value = p.caption || '';

  $('#edit-genres').innerHTML = genres.map(g => `
    <label><input type="checkbox" value="${escapeHtml(g.slug)}" ${p.genres.includes(g.slug) ? 'checked' : ''}> ${escapeHtml(g.name)}</label>
  `).join('') || '<em>No genres defined yet.</em>';

  $('#edit-collections').innerHTML = collections.map(c => `
    <label><input type="checkbox" value="${escapeHtml(c.slug)}" ${p.collections.includes(c.slug) ? 'checked' : ''}> ${escapeHtml(c.name)}</label>
  `).join('') || '<em>No collections defined yet.</em>';

  renderCoverActions(p);

  $('#edit-delete').onclick = async () => {
    if (!confirm('Delete this photo and its R2 objects?')) return;
    await api(`/photos/${encodeURIComponent(p.id)}`, { method: 'DELETE' });
    dlg.close('deleted');
    refresh();
  };

  dlg.showModal();
}

function renderCoverActions(p) {
  const box = $('#edit-covers');
  if (!genres.length && !collections.length) {
    box.innerHTML = '<em>Create a genre or collection first.</em>';
    return;
  }
  const row = (label, isCover, isMember, kind, slug) => `
    <div class="cover-row">
      <span>${escapeHtml(label)}${isMember ? '' : ' <em>(not assigned)</em>'} ${isCover ? '<strong>(current cover)</strong>' : ''}</span>
      <button type="button" data-kind="${kind}" data-slug="${escapeHtml(slug)}" data-set="${isCover ? '0' : '1'}">
        ${isCover ? 'Remove cover' : 'Set as cover'}
      </button>
    </div>`;
  box.innerHTML = [
    ...genres.map(g => row(`Genre · ${g.name}`, g.cover === p.id, p.genres.includes(g.slug), 'genres', g.slug)),
    ...collections.map(c => row(`Collection · ${c.name}`, c.cover === p.id, p.collections.includes(c.slug), 'collections', c.slug)),
  ].join('');

  box.querySelectorAll('button').forEach(btn => {
    btn.addEventListener('click', async () => {
      const { kind, slug, set } = btn.dataset;
      btn.disabled = true;
      try {
        await api(`/${kind}/${encodeURIComponent(slug)}/cover`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ cover: set === '1' ? p.id : '' }),
        });
        if (kind === 'genres') {
          const g = genres.find(x => x.slug === slug);
          if (g) g.cover = set === '1' ? p.id : '';
        } else {
          const c = collections.find(x => x.slug === slug);
          if (c) c.cover = set === '1' ? p.id : '';
        }
        renderCoverActions(p);
      } catch (err) {
        alert('Set cover failed: ' + err.message);
        btn.disabled = false;
      }
    });
  });
}

$('#edit-photo').addEventListener('close', async (e) => {
  const dlg = e.currentTarget;
  if (dlg.returnValue !== 'save') return;
  const id = $('[name=id]', dlg).value;
  const caption = $('[name=caption]', dlg).value;
  const selectedGenres = $$('#edit-genres input:checked').map(i => i.value);
  const selectedCols = $$('#edit-collections input:checked').map(i => i.value);
  await api(`/photos/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ caption, genres: selectedGenres, collections: selectedCols })
  });
  refresh();
});

$('#upload-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const fd = new FormData(e.target);
  const status = $('#upload-status');
  status.textContent = 'Uploading…';
  try {
    const r = await fetch('/api/photos', { method: 'POST', body: fd });
    if (!r.ok) throw new Error(`${r.status}`);
    status.textContent = 'Done';
    e.target.reset();
    refresh();
  } catch (err) {
    status.textContent = 'Error: ' + err.message;
  }
});

$('#genre-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const data = Object.fromEntries(new FormData(e.target));
  await api('/genres', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });
  e.target.reset();
  refresh();
});

$('#collection-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const data = Object.fromEntries(new FormData(e.target));
  await api('/collections', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });
  e.target.reset();
  refresh();
});

$$('.tab').forEach(btn => {
  btn.addEventListener('click', () => {
    $$('.tab').forEach(b => b.classList.remove('active'));
    $$('.panel').forEach(p => p.classList.remove('active'));
    btn.classList.add('active');
    $(`#${btn.dataset.tab}`).classList.add('active');
  });
});

refresh().catch(err => alert('Load failed: ' + err.message));
