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

async function refresh() {
  [photos, genres, collections] = await Promise.all([
    api('/photos'), api('/genres'), api('/collections')
  ]);
  renderPhotos();
  renderGenres();
  renderCollections();
}

function renderPhotos() {
  const grid = $('#photo-grid');
  grid.innerHTML = '';
  [...photos].reverse().forEach(p => {
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

  $('#edit-delete').onclick = async () => {
    if (!confirm('Delete this photo and its R2 objects?')) return;
    await api(`/photos/${encodeURIComponent(p.id)}`, { method: 'DELETE' });
    dlg.close('deleted');
    refresh();
  };

  dlg.showModal();
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
