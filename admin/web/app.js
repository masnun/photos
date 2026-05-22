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
let selectMode = false;
let selected = new Set();

async function refresh() {
  [photos, genres, collections] = await Promise.all([
    api('/photos'), api('/genres'), api('/collections')
  ]);
  pruneSelection();
  renderFilter();
  renderPhotos();
  renderGenres();
  renderCollections();
  renderBulkBar();
}

function pruneSelection() {
  const ids = new Set(photos.map(p => p.id));
  for (const id of [...selected]) if (!ids.has(id)) selected.delete(id);
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

function selectedTaxonomy() {
  if (collectionFilter !== 'all' && collectionFilter !== 'untagged')
    return collections.find(c => c.slug === collectionFilter) || null;
  if (genreFilter !== 'all' && genreFilter !== 'untagged')
    return genres.find(g => g.slug === genreFilter) || null;
  return null;
}

function displayedPhotos() {
  const list = photos.filter(photoMatchesFilter);
  const tax = selectedTaxonomy();
  if (tax) return orderedMembers(list, tax.order);
  return [...list].sort((a, b) => {
    const ad = a.taken_at || a.uploaded_at || '';
    const bd = b.taken_at || b.uploaded_at || '';
    if (ad === bd) return 0;
    return ad < bd ? 1 : -1;
  });
}

function renderPhotos() {
  const grid = $('#photo-grid');
  grid.innerHTML = '';
  displayedPhotos().forEach(p => {
    const el = document.createElement('div');
    el.className = 'photo-card';
    if (selectMode && selected.has(p.id)) el.classList.add('selected');
    const genreTags = p.genres.map(t => `<span class="pill pill-genre">${escapeHtml(t)}</span>`).join('');
    const colTags = p.collections.map(t => {
      const cls = t === 'featured' ? 'pill pill-featured' : 'pill pill-collection';
      return `<span class="${cls}">${escapeHtml(t)}</span>`;
    }).join('');
    const tags = genreTags + colTags;
    const caption = p.caption ? `<div class="caption">${escapeHtml(p.caption)}</div>` : '';
    el.innerHTML = `
      <div class="photo-frame"><img src="${escapeHtml(p.urls.thumb)}" alt=""></div>
      <div class="meta">
        ${caption}
        <div class="tags">${tags || '<em>untagged</em>'}</div>
      </div>`;
    el.addEventListener('click', () => {
      if (selectMode) {
        if (selected.has(p.id)) selected.delete(p.id); else selected.add(p.id);
        el.classList.toggle('selected');
        updateSelectCount();
      } else {
        openEdit(p);
      }
    });
    grid.appendChild(el);
  });
}

function renderBulkBar() {
  const bg = $('#bulk-genre');
  const bc = $('#bulk-collection');
  bg.innerHTML = '<option value="">Add to genre…</option>' +
    genres.map(g => `<option value="${escapeHtml(g.slug)}">${escapeHtml(g.name)}</option>`).join('');
  bc.innerHTML = '<option value="">Add to collection…</option>' +
    collections.map(c => `<option value="${escapeHtml(c.slug)}">${escapeHtml(c.name)}</option>`).join('');
  updateSelectCount();
}

function updateSelectCount() {
  const count = $('#select-count');
  const actions = $('#bulk-actions');
  if (selectMode) {
    count.hidden = false;
    actions.hidden = selected.size === 0;
    count.textContent = `${selected.size} selected`;
  } else {
    count.hidden = true;
    actions.hidden = true;
  }
}

async function bulkAdd(kind, slug) {
  if (!slug || selected.size === 0) return;
  const ids = [...selected];
  const targets = photos.filter(p => ids.includes(p.id));
  for (const p of targets) {
    const list = kind === 'genres' ? p.genres : p.collections;
    if (list.includes(slug)) continue;
    const next = [...list, slug];
    const body = kind === 'genres' ? { genres: next } : { collections: next };
    await api(`/photos/${encodeURIComponent(p.id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  }
  await refresh();
}

function renderGenres() {
  const ul = $('#genre-list');
  ul.innerHTML = '';
  genres.forEach(g => {
    const li = document.createElement('li');
    renderGenreRow(li, g);
    ul.appendChild(li);
  });
}

function renderGenreRow(li, g) {
  li.classList.remove('edit-row');
  li.innerHTML = `<strong>${escapeHtml(g.name)}</strong> <code>${escapeHtml(g.slug)}</code>
    <span>${escapeHtml(g.description || '')}</span>
    <button class="reorder-btn">Reorder</button>
    <button class="edit-btn">Edit</button>
    <button class="danger">Delete</button>`;
  li.querySelector('.reorder-btn').addEventListener('click', () => openReorder('genres', g));
  li.querySelector('.edit-btn').addEventListener('click', () => renderGenreEdit(li, g));
  li.querySelector('.danger').addEventListener('click', async () => {
    if (!confirm(`Delete genre "${g.slug}"? Photos referencing it will keep the dangling slug.`)) return;
    await api(`/genres/${encodeURIComponent(g.slug)}`, { method: 'DELETE' });
    refresh();
  });
}

function renderGenreEdit(li, g) {
  li.classList.add('edit-row');
  li.innerHTML = `
    <code>${escapeHtml(g.slug)}</code>
    <input class="edit-name" value="${escapeHtml(g.name)}" placeholder="Name">
    <textarea class="edit-desc" rows="3" placeholder="Description (Markdown supported)">${escapeHtml(g.description || '')}</textarea>
    <button class="save-btn">Save</button>
    <button class="cancel-btn">Cancel</button>`;
  const nameInput = li.querySelector('.edit-name');
  const descInput = li.querySelector('.edit-desc');
  li.querySelector('.save-btn').addEventListener('click', async () => {
    const payload = {
      slug: g.slug,
      name: nameInput.value.trim() || g.name,
      description: descInput.value,
      cover: g.cover || '',
    };
    await api('/genres', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    refresh();
  });
  li.querySelector('.cancel-btn').addEventListener('click', () => renderGenreRow(li, g));
  nameInput.focus();
}

function renderCollections() {
  const ul = $('#collection-list');
  ul.innerHTML = '';
  const featured = collections.find(c => c.slug === 'featured');
  const rest = collections.filter(c => c.slug !== 'featured');
  for (const c of featured ? [featured, ...rest] : rest) {
    const li = document.createElement('li');
    renderCollectionRow(li, c);
    ul.appendChild(li);
  }
}

function renderCollectionRow(li, c) {
  const isFeatured = c.slug === 'featured';
  li.innerHTML = `<strong>${escapeHtml(c.name)}</strong> <code>${escapeHtml(c.slug)}</code>
    <span>${escapeHtml(c.description || '')}</span>
    <button class="reorder-btn">Reorder</button>
    ${isFeatured ? '' : `<button class="edit-btn">Edit</button>
    <button class="danger">Delete</button>`}`;
  li.querySelector('.reorder-btn').addEventListener('click', () => openReorder('collections', c));
  if (isFeatured) return;
  li.querySelector('.edit-btn').addEventListener('click', () => renderCollectionEdit(li, c));
  li.querySelector('.danger').addEventListener('click', async () => {
    if (!confirm(`Delete collection "${c.slug}"? Photos referencing it will keep the dangling slug.`)) return;
    await api(`/collections/${encodeURIComponent(c.slug)}`, { method: 'DELETE' });
    refresh();
  });
}

function renderCollectionEdit(li, c) {
  li.classList.add('edit-row');
  li.innerHTML = `
    <code>${escapeHtml(c.slug)}</code>
    <input class="edit-name" value="${escapeHtml(c.name)}" placeholder="Name">
    <textarea class="edit-desc" rows="3" placeholder="Description (Markdown supported)">${escapeHtml(c.description || '')}</textarea>
    <label class="edit-hero"><input type="checkbox" class="edit-hero-cb" ${c.hero ? 'checked' : ''}> Featured</label>
    <button class="save-btn">Save</button>
    <button class="cancel-btn">Cancel</button>`;
  const nameInput = li.querySelector('.edit-name');
  const descInput = li.querySelector('.edit-desc');
  const heroInput = li.querySelector('.edit-hero-cb');
  li.querySelector('.save-btn').addEventListener('click', async () => {
    const payload = {
      slug: c.slug,
      name: nameInput.value.trim() || c.name,
      description: descInput.value,
      cover: c.cover || '',
      hero: heroInput.checked,
    };
    await api('/collections', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    refresh();
  });
  li.querySelector('.cancel-btn').addEventListener('click', () => {
    li.classList.remove('edit-row');
    renderCollectionRow(li, c);
  });
  nameInput.focus();
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
  const memberGenres = genres.filter(g => p.genres.includes(g.slug));
  const memberCols = collections.filter(c => p.collections.includes(c.slug));
  if (!memberGenres.length && !memberCols.length) {
    box.innerHTML = '<em>Assign this photo to a genre or collection to set it as a cover.</em>';
    return;
  }
  const row = (label, isCover, kind, slug) => `
    <div class="cover-row">
      <span>${escapeHtml(label)} ${isCover ? '<strong>(current cover)</strong>' : ''}</span>
      <button type="button" data-kind="${kind}" data-slug="${escapeHtml(slug)}" data-set="${isCover ? '0' : '1'}">
        ${isCover ? 'Remove cover' : 'Set as cover'}
      </button>
    </div>`;
  box.innerHTML = [
    ...memberGenres.map(g => row(`Genre · ${g.name}`, g.cover === p.id, 'genres', g.slug)),
    ...memberCols.map(c => row(`Collection · ${c.name}`, c.cover === p.id, 'collections', c.slug)),
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

$('#select-toggle').addEventListener('click', () => {
  selectMode = !selectMode;
  $('#select-toggle').classList.toggle('active', selectMode);
  $('#select-toggle').textContent = selectMode ? 'Done' : 'Select';
  if (!selectMode) selected.clear();
  renderPhotos();
  updateSelectCount();
});

$('#bulk-clear').addEventListener('click', () => {
  selected.clear();
  renderPhotos();
  updateSelectCount();
});

$('#bulk-genre').addEventListener('change', async (e) => {
  const slug = e.target.value;
  e.target.value = '';
  await bulkAdd('genres', slug);
});

$('#bulk-collection').addEventListener('change', async (e) => {
  const slug = e.target.value;
  e.target.value = '';
  await bulkAdd('collections', slug);
});

$$('.tab').forEach(btn => {
  btn.addEventListener('click', () => {
    $$('.tab').forEach(b => b.classList.remove('active'));
    $$('.panel').forEach(p => p.classList.remove('active'));
    btn.classList.add('active');
    $(`#${btn.dataset.tab}`).classList.add('active');
  });
});

// ---- Reorder ----
let reorderKind = null;   // 'genres' | 'collections'
let reorderSlug = null;

function photoDateAsc(a, b) {
  const ad = a.taken_at || a.uploaded_at || '';
  const bd = b.taken_at || b.uploaded_at || '';
  if (ad === bd) return 0;
  return ad < bd ? -1 : 1;
}

// Mirror the generator: ordered IDs first, then unordered photos oldest-first.
function orderedMembers(members, order) {
  if (!order || !order.length) return [...members].sort(photoDateAsc);
  const idx = new Map(order.map((id, i) => [id, i]));
  const known = members.filter(p => idx.has(p.id)).sort((a, b) => idx.get(a.id) - idx.get(b.id));
  const unknown = members.filter(p => !idx.has(p.id)).sort(photoDateAsc);
  return [...known, ...unknown];
}

function openReorder(kind, tax) {
  reorderKind = kind;
  reorderSlug = tax.slug;
  const listKey = kind === 'genres' ? 'genres' : 'collections';
  const members = photos.filter(p => (p[listKey] || []).includes(tax.slug));
  const ordered = orderedMembers(members, tax.order);

  $('#reorder-title').textContent = `Reorder · ${tax.name}`;
  const grid = $('#reorder-grid');
  grid.innerHTML = '';
  if (!ordered.length) {
    grid.innerHTML = '<p class="empty">No photos in this group yet.</p>';
  }
  ordered.forEach(p => {
    const el = document.createElement('div');
    el.className = 'reorder-card';
    el.draggable = true;
    el.dataset.id = p.id;
    el.innerHTML = `<img src="${escapeHtml(p.urls.thumb)}" alt="" draggable="false">`;
    wireDrag(el, grid);
    grid.appendChild(el);
  });
  $('#reorder-dialog').showModal();
}

let dragEl = null;

function wireDrag(el, grid) {
  el.addEventListener('dragstart', () => {
    dragEl = el;
    el.classList.add('dragging');
  });
  el.addEventListener('dragend', () => {
    el.classList.remove('dragging');
    dragEl = null;
  });
  el.addEventListener('dragover', (e) => {
    e.preventDefault();
    if (!dragEl || dragEl === el) return;
    const rect = el.getBoundingClientRect();
    const after = (e.clientY - rect.top) > rect.height / 2 ||
      ((e.clientY - rect.top) > 0 && (e.clientX - rect.left) > rect.width / 2);
    grid.insertBefore(dragEl, after ? el.nextSibling : el);
  });
}

$('#reorder-cancel').addEventListener('click', () => $('#reorder-dialog').close());

$('#reorder-save').addEventListener('click', async () => {
  const order = $$('#reorder-grid .reorder-card').map(el => el.dataset.id);
  try {
    await api(`/${reorderKind}/${encodeURIComponent(reorderSlug)}/order`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ order }),
    });
    $('#reorder-dialog').close();
    refresh();
  } catch (err) {
    alert('Save order failed: ' + err.message);
  }
});

refresh().catch(err => alert('Load failed: ' + err.message));
