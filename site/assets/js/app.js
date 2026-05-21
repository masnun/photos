async function loadManifest() {
  const r = await fetch('/data/photos.json', { cache: 'no-cache' });
  return r.json();
}

function escapeHtml(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
}

function renderTaxonomy(sel, items, photos, kind) {
  const wrap = document.querySelector(sel);
  if (!items.length) { wrap.innerHTML = '<p class="empty">Nothing here yet.</p>'; return; }
  const href = kind === 'genre' ? '/genre/' : '/collection/';
  wrap.innerHTML = items.map(it => {
    let cover = null;
    if (kind === 'collection' && it.cover) {
      cover = photos.find(p => p.id === it.cover);
    }
    if (!cover) {
      const key = kind === 'genre' ? 'genres' : 'collections';
      cover = photos.find(p => p[key].includes(it.slug));
    }
    const img = cover ? `<img src="${escapeHtml(cover.urls.thumb)}" alt="" loading="lazy">` : '<div class="tile-empty"></div>';
    const desc = it.description ? `<span>${escapeHtml(it.description)}</span>` : '';
    return `<a class="tile" href="${href}?s=${encodeURIComponent(it.slug)}">
      ${img}
      <div class="tile-label"><strong>${escapeHtml(it.name)}</strong>${desc}</div>
    </a>`;
  }).join('');
}

function renderGallery(sel, photos) {
  const wrap = document.querySelector(sel);
  if (!photos.length) { wrap.innerHTML = '<p class="empty">No photos yet.</p>'; return; }
  wrap.innerHTML = photos.map(p => `
    <a class="gallery-item" href="/photo/?id=${encodeURIComponent(p.id)}">
      <img src="${escapeHtml(p.urls.thumb)}" alt="${escapeHtml(p.caption || '')}" loading="lazy">
    </a>
  `).join('');
}

function renderPhoto(sel, p, m) {
  const wrap = document.querySelector(sel);
  const genreLinks = p.genres.map(slug => {
    const g = m.genres.find(x => x.slug === slug);
    return g ? `<a href="/genre/?s=${encodeURIComponent(slug)}">${escapeHtml(g.name)}</a>` : '';
  }).filter(Boolean).join(', ');
  const colLinks = p.collections.map(slug => {
    const c = m.collections.find(x => x.slug === slug);
    return c ? `<a href="/collection/?s=${encodeURIComponent(slug)}">${escapeHtml(c.name)}</a>` : '';
  }).filter(Boolean).join(', ');
  const exif = renderExif(p);
  const taken = p.taken_at
    ? new Date(p.taken_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })
    : '';
  wrap.innerHTML = `
    <figure class="photo-detail">
      <img src="${escapeHtml(p.urls.web)}" alt="${escapeHtml(p.caption || '')}">
      ${p.caption ? `<figcaption>${escapeHtml(p.caption)}</figcaption>` : ''}
    </figure>
    <div class="photo-meta">
      ${taken ? `<p><strong>Taken:</strong> ${escapeHtml(taken)}</p>` : ''}
      ${genreLinks ? `<p><strong>Genres:</strong> ${genreLinks}</p>` : ''}
      ${colLinks ? `<p><strong>Collections:</strong> ${colLinks}</p>` : ''}
      ${exif}
      <p><a href="${escapeHtml(p.urls.full)}" target="_blank" rel="noopener">View full size</a></p>
    </div>
  `;
}

function renderExif(p) {
  const e = p.exif;
  if (!e) return '';
  const rows = [];
  if (e.camera) rows.push(['Camera', e.camera]);
  if (e.lens) rows.push(['Lens', e.lens]);
  const settings = [e.focal_length, e.aperture, e.shutter, e.iso ? `ISO ${e.iso}` : ''].filter(Boolean).join(' · ');
  if (settings) rows.push(['Settings', settings]);
  if (!rows.length) return '';
  return `<dl class="exif">${rows.map(([k, v]) => `<dt>${k}</dt><dd>${escapeHtml(v)}</dd>`).join('')}</dl>`;
}
