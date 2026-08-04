const state = { snapshot: null, queueSignature: '', session: null, pendingSignup: null, source: null, stations: [], playlists: [], enrichmentTitle: '', enrichmentGeneration: 0, artworkURL: '', setupStep: 0 };
const $ = selector => document.querySelector(selector);

function cookie(name) {
  return document.cookie.split('; ').find(value => value.startsWith(`${name}=`))?.split('=').slice(1).join('=') || '';
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  const csrf = cookie('jukebox_csrf');
  if (csrf && !['GET', 'HEAD'].includes(options.method || 'GET')) headers.set('X-CSRF-Token', decodeURIComponent(csrf));
  const response = await fetch(path, { ...options, headers });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error?.message || `Request failed (${response.status})`);
  return body;
}

function showToast(message) {
  const toast = $('#toast'); toast.textContent = message; toast.hidden = false;
  clearTimeout(showToast.timer); showToast.timer = setTimeout(() => { toast.hidden = true; }, 4500);
}

function formatTime(value) {
  if (!Number.isFinite(value) || value < 0) return '0:00';
  const minutes = Math.floor(value / 60); return `${minutes}:${String(Math.floor(value % 60)).padStart(2, '0')}`;
}

function render(snapshot) {
  state.snapshot = snapshot;
  const playback = snapshot.playback || {};
  const current = snapshot.items.find(item => item.id === playback.current_item_id) || snapshot.items.find(item => item.status === 'current');
  const displayTitle = playback.title || (current ? sourceLabel(current.source.url) : 'Nothing playing');
  $('#now-playing-heading').textContent = displayTitle;
  $('#now-source').textContent = current ? `${current.source.kind === 'direct' ? 'Stream' : current.source.kind} · ${current.source.url}` : 'Add a radio stream or audio URL to get started.';
  $('#position').textContent = formatTime(playback.position_seconds);
  $('#duration').textContent = playback.duration_seconds > 0 ? formatTime(playback.duration_seconds) : 'Live';
  const progress = $('#playback-progress'); progress.max = playback.duration_seconds || 1; progress.value = playback.duration_seconds ? Math.min(playback.position_seconds || 0, playback.duration_seconds) : 0;
  $('#volume').value = playback.volume ?? 100; $('#volume-output').value = playback.volume ?? 100;
  const statusText = playback.error ? `${playback.status}: ${playback.error}` : playback.buffering ? 'Buffering…' : playback.status || 'idle';
  $('#playback-state').textContent = statusText.charAt(0).toUpperCase() + statusText.slice(1);
  const hasCurrent = Boolean(current);
  $('#pause-button').disabled = !hasCurrent || playback.paused;
  $('#resume-button').disabled = !hasCurrent || (!playback.paused && playback.status !== 'stopped');
  $('#skip-button').disabled = !hasCurrent;
  const vote = snapshot.skip_vote; const voteStatus = $('#vote-status');
  if (vote?.enabled && hasCurrent) {
    voteStatus.hidden = false; voteStatus.textContent = vote.voted ? `Your vote is in · ${vote.votes} of ${vote.required} needed` : `${vote.votes} of ${vote.required} skip votes · ${vote.active_listeners} active listener${vote.active_listeners === 1 ? '' : 's'}`;
    $('#skip-button').textContent = vote.voted ? '✓' : '›|'; $('#skip-button').setAttribute('aria-label', vote.voted ? 'Withdraw skip vote' : 'Vote to skip current item');
  } else { voteStatus.hidden = true; $('#skip-button').textContent = '›|'; $('#skip-button').setAttribute('aria-label', 'Skip current item'); }
  renderQueue(snapshot.items);
  if (playback.title && playback.title !== state.enrichmentTitle) {
    state.enrichmentTitle = playback.title;
    state.enrichmentGeneration += 1;
    resetNowArtwork(true);
    loadEnrichment(playback.title, renderNowEnrichment, 0, state.enrichmentGeneration);
  }
  if (!playback.title) { state.enrichmentTitle = ''; state.enrichmentGeneration += 1; $('#artist-panel').hidden = true; resetNowArtwork(false); }
}

async function loadEnrichment(title, renderer, attempt = 0, generation = null) {
  try {
    const body = await api(`/api/v1/enrichment?title=${encodeURIComponent(title)}`);
    if (generation !== null && (generation !== state.enrichmentGeneration || title !== state.enrichmentTitle)) return;
    renderer(body.enrichment || {}, generation);
    if (body.enrichment?.status === 'pending' && attempt < 24) {
      const delay = Math.min(750 + (attempt * 150), 2000);
      setTimeout(() => loadEnrichment(title, renderer, attempt + 1, generation), delay);
    }
  }
  catch (error) { if (attempt === 0) console.debug('Artist metadata unavailable', error.message); }
}

function resetNowArtwork(loading) {
  const hero = $('.artwork'); const section = $('.now-playing');
  state.artworkURL = '';
  hero.style.removeProperty('--artwork-image'); section.style.removeProperty('--now-artwork');
  hero.classList.remove('has-image'); section.classList.remove('has-artwork');
  hero.classList.toggle('is-loading', loading);
  hero.setAttribute('aria-hidden', 'true'); hero.removeAttribute('role'); hero.removeAttribute('aria-label');
}

function loadNowArtwork(image, artist, generation, attempt = 0) {
  if (!image?.url) { $('.artwork').classList.remove('is-loading'); return; }
  const preload = new Image();
  preload.onload = () => {
    if (generation !== state.enrichmentGeneration) return;
    const escaped = image.url.replaceAll('"', '%22'); const cssImage = `url("${escaped}")`;
    const hero = $('.artwork'); const section = $('.now-playing');
    state.artworkURL = image.url; hero.style.setProperty('--artwork-image', cssImage); section.style.setProperty('--now-artwork', cssImage);
    hero.classList.add('has-image'); hero.classList.remove('is-loading'); section.classList.add('has-artwork');
    hero.removeAttribute('aria-hidden'); hero.setAttribute('role', 'img'); hero.setAttribute('aria-label', `${artist || 'Artist'} photo`);
  };
  preload.onerror = () => {
    if (generation !== state.enrichmentGeneration) return;
    if (attempt >= 19) { $('.artwork').classList.remove('is-loading'); return; }
    setTimeout(() => { if (generation === state.enrichmentGeneration) loadNowArtwork(image, artist, generation, attempt + 1); }, 1500);
  };
  preload.src = image.url;
}

function renderNowEnrichment(value, generation = state.enrichmentGeneration) {
  const panel = $('#artist-panel'); if (value.status !== 'ready') { panel.hidden = true; return; }
  panel.hidden = false; const artist = value.hint?.artist || ''; const artistButton = $('#artist-name'); artistButton.textContent = artist; artistButton.onclick = () => discover(artist);
  const genres = $('#artist-genres'); genres.replaceChildren(); (value.genres || []).slice(0, 8).forEach(name => { const tag = document.createElement('button'); tag.type = 'button'; tag.textContent = name; tag.setAttribute('aria-label', `Discover ${name} music`); tag.addEventListener('click', () => discoverGenre(name)); genres.append(tag); }); $('#artist-bio').textContent = value.biography || '';
  const related = $('#related-artists'); related.replaceChildren(); if (value.related_artists?.length) { const label = document.createElement('span'); label.textContent = 'Related '; related.append(label); value.related_artists.slice(0, 6).forEach(item => { const button = document.createElement('button'); button.type = 'button'; button.textContent = item.name; button.addEventListener('click', () => discover(item.name)); related.append(button); }); }
  loadNowArtwork(value.image, artist, generation);
  const attribution = $('#artist-attribution'); attribution.textContent = value.image?.attribution || (value.provider ? `Metadata via ${value.provider}` : ''); attribution.href = value.image?.source_url || value.artist_url || '#';
}

function sourceLabel(url) {
  try { const parsed = new URL(url); return parsed.pathname.split('/').filter(Boolean).pop() || parsed.hostname; } catch { return url; }
}

function renderQueue(items) {
  const signature = JSON.stringify(items.map(item => [item.id, item.title, item.source.kind, item.source.url, item.submitter.kind, item.submitter.username, item.submitter.display_name, item.position, item.status, item.error]));
  if (signature === state.queueSignature) return;
  state.queueSignature = signature;
  const list = $('#queue-list'); list.replaceChildren(); $('#queue-empty').hidden = items.length > 0; $('#clear-button').hidden = items.length === 0;
  items.forEach((item, index) => {
    const row = document.createElement('li'); row.className = `queue-item ${item.status}`;
    const number = document.createElement('span'); number.className = 'queue-number'; number.textContent = item.status === 'current' ? '▶' : String(index + 1).padStart(2, '0');
    const copy = document.createElement('div'); copy.className = 'queue-copy';
    const title = document.createElement('span'); title.className = 'queue-title'; title.textContent = item.title || sourceLabel(item.source.url); title.title = item.source.url;
    const meta = document.createElement('div'); meta.className = 'queue-meta';
    const submitter = item.submitter.kind === 'user' ? item.submitter.username : item.submitter.display_name || 'Anonymous';
    meta.textContent = item.error ? `${submitter} · ${item.error}` : `Added by ${submitter}`; const details = document.createElement('div'); details.className = 'queue-copy-details'; details.append(title, meta); copy.append(details); if (item.title) { const loading = document.createElement('span'); loading.className = 'queue-artwork-loading'; loading.setAttribute('aria-label', `Loading artist information for ${item.title}`); copy.prepend(loading); loadEnrichment(item.title, value => renderQueueEnrichment(copy, details, value)); }
    const badge = document.createElement('span'); badge.className = 'queue-badge'; badge.textContent = item.status;
    const actions = document.createElement('div'); actions.className = 'item-actions';
    if (index > 0) actions.append(actionButton('↑', `Move ${title.textContent} up`, () => moveItem(index, -1)));
    if (index < items.length - 1) actions.append(actionButton('↓', `Move ${title.textContent} down`, () => moveItem(index, 1)));
    actions.append(actionButton('×', `Remove ${title.textContent}`, () => removeItem(item.id)));
    row.append(number, copy, badge, actions); list.append(row);
  });
}

function renderQueueEnrichment(copy, details, value) {
  if (!copy.isConnected || value.status === 'pending') return;
  if (value.status !== 'ready') { copy.querySelector('.queue-artwork-loading')?.remove(); return; }
  copy.querySelector('.queue-artwork-loading')?.remove(); copy.querySelector('.queue-artwork')?.remove(); details.querySelector('.queue-enrichment')?.remove();
  if (value.image?.url) { const image = document.createElement('img'); image.className = 'queue-artwork'; image.src = value.image.url; image.alt = `${value.hint?.artist || 'Artist'} photo`; copy.prepend(image); }
  const context = document.createElement('div'); context.className = 'queue-enrichment';
  if (value.hint?.artist) { const artist = document.createElement('button'); artist.type = 'button'; artist.textContent = value.hint.artist; artist.addEventListener('click', () => discover(value.hint.artist)); context.append(artist); }
  const genres = document.createElement('span'); genres.className = 'queue-genres'; (value.genres || []).slice(0, 4).forEach(name => { const tag = document.createElement('button'); tag.type = 'button'; tag.textContent = name; tag.addEventListener('click', () => discoverGenre(name)); genres.append(tag); }); context.append(genres); details.append(context);
}

function actionButton(text, label, handler) { const button = document.createElement('button'); button.type = 'button'; button.className = 'icon-button'; button.textContent = text; button.setAttribute('aria-label', label); button.addEventListener('click', handler); return button; }
function revisionHeader() { return { 'If-Match': `"${state.snapshot?.revision ?? 0}"` }; }
async function removeItem(id) { try { render(await api(`/api/v1/queue/items/${id}`, { method: 'DELETE', headers: revisionHeader() })); } catch (error) { showToast(error.message); refresh(); } }
async function moveItem(index, delta) { const ids = state.snapshot.items.map(item => item.id); [ids[index], ids[index + delta]] = [ids[index + delta], ids[index]]; try { render(await api('/api/v1/queue/order', { method: 'PUT', headers: revisionHeader(), body: JSON.stringify({ item_ids: ids }) })); } catch (error) { showToast(error.message); refresh(); } }
async function control(path, options = {}) { try { render(await api(path, { method: 'POST', ...options })); } catch (error) { showToast(error.message); } }
async function refresh() { try { render(await api('/api/v1/queue')); } catch (error) { showToast(error.message); } }

async function loadAutoQueueStatus() {
  try {
    const value = await api('/api/v1/autoqueue'); const panel = $('#autoqueue-panel'); const button = $('#autoqueue-toggle'); const mode = $('#autoqueue-mode'); const seeds = $('#autoqueue-seeds');
    panel.hidden = !value.available; if (!value.available) return;
    button.dataset.enabled = String(Boolean(value.enabled)); button.setAttribute('aria-pressed', String(Boolean(value.enabled))); button.innerHTML = `<span aria-hidden="true">✦</span> Auto-queue ${value.enabled ? 'on' : 'off'}`;
    mode.value = value.mode || 'active_users'; seeds.hidden = mode.value !== 'specific_seeds'; $('#autoqueue-artists').value = value.artists || ''; $('#autoqueue-genres').value = value.genres || '';
    const descriptions = { active_users:'fairly rotating through active listeners', specific_seeds:'your selected artists and genres', related_last:'artists and genres related to the last queued item' };
    $('#autoqueue-status').textContent = value.enabled ? `Keeping ${value.depth || 3} track${String(value.depth) === '1' ? '' : 's'} ahead using ${descriptions[mode.value]}.` : `Ready to fill the queue using ${descriptions[mode.value]}.`;
  } catch { $('#autoqueue-panel').hidden = true; }
}

function connectEvents() {
  state.source?.close(); const connection = $('#connection-status'); state.source = new EventSource('/api/v1/events');
  state.source.addEventListener('open', () => { connection.className = 'connection online'; connection.lastChild.textContent = ' Connected'; });
  state.source.addEventListener('queue', event => { try { render(JSON.parse(event.data)); } catch { showToast('Received an invalid player update.'); } });
  state.source.addEventListener('error', () => { connection.className = 'connection offline'; connection.lastChild.textContent = ' Reconnecting'; });
}

async function loadSession() {
  try { const result = await api('/api/v1/auth/session'); state.session = result.authenticated ? result.session : null; renderIdentity(); await loadLibrary(); } catch { state.session = null; renderIdentity(); await loadLibrary(); }
}

function renderIdentity() {
  const button = $('#account-button'); const footer = $('#footer-identity');
  if (state.session) { button.textContent = `${state.session.user.username} · Log out`; footer.textContent = `Signed in as ${state.session.user.username}`; }
  else { button.textContent = 'Log in'; footer.textContent = 'Browsing anonymously'; }
  document.querySelectorAll('[data-auth-nav]').forEach(link => { link.hidden = !state.session; });
  document.querySelectorAll('[data-admin-nav]').forEach(link => { link.hidden = !state.session?.user?.is_admin; });
}

function resetAuth() { $('#login-form').hidden = false; $('#signup-form').hidden = true; $('#login-error').textContent = ''; $('#signup-error').textContent = ''; state.pendingSignup = null; }

async function loadLibrary() {
  const query = $('#library-search').value.trim(); const suffix = query ? `?q=${encodeURIComponent(query)}` : '';
  try {
    const stations = await api(`/api/v1/stations${suffix}`); state.stations = stations.stations || [];
    const history = await api(`/api/v1/history${suffix}`); renderHistory(history.history || []);
    if (state.session) { const playlists = await api(`/api/v1/playlists${suffix}`); state.playlists = playlists.playlists || []; }
    else state.playlists = [];
    renderLibrary();
  } catch (error) { showToast(error.message); }
}

function renderLibrary() {
  const stationList = $('#station-list'); stationList.replaceChildren();
  state.stations.forEach(station => {
    const card = document.createElement('article'); card.className = 'station-card';
    const icon = document.createElement('span'); icon.className = 'station-icon'; icon.textContent = '◉'; icon.setAttribute('aria-hidden', 'true');
    const copy = document.createElement('div'); copy.className = 'station-copy'; const name = document.createElement('strong'); name.textContent = station.name; const url = document.createElement('small'); url.textContent = station.stream_url; copy.append(name, url);
    const actions = document.createElement('div'); actions.className = 'station-actions';
    actions.append(actionButton('▶', `Play ${station.name}`, () => queueStation(station)));
    actions.lastChild.classList.add('play-station');
    if (state.session) actions.append(actionButton(station.favorite ? '★' : '☆', `${station.favorite ? 'Remove' : 'Add'} ${station.name} ${station.favorite ? 'from' : 'to'} favorites`, () => toggleFavorite(station)));
    if (station.owner_user_id) actions.append(actionButton('×', `Delete ${station.name}`, () => deleteStation(station)));
    card.append(icon, copy, actions); stationList.append(card);
  });
  if (!state.stations.length) { const note = document.createElement('p'); note.className = 'library-note'; note.textContent = 'No stations match this search.'; stationList.append(note); }
  $('#personal-library').hidden = !state.session; $('#library-login-note').hidden = Boolean(state.session);
  const playlists = $('#playlist-list'); playlists.replaceChildren();
  state.playlists.forEach(playlist => {
    const card = document.createElement('div'); card.className = 'playlist-card'; const copy = document.createElement('span'); const strong = document.createElement('strong'); strong.textContent = playlist.name; const count = document.createElement('small'); count.textContent = `${playlist.items.length} item${playlist.items.length === 1 ? '' : 's'}`; copy.append(strong, document.createElement('br'), count);
    const actions = document.createElement('div'); actions.className = 'item-actions'; if (playlist.items.length) actions.append(actionButton('▶', `Queue ${playlist.name}`, () => queuePlaylist(playlist))); actions.append(actionButton('×', `Delete ${playlist.name}`, () => deletePlaylist(playlist))); card.append(copy, actions); playlists.append(card);
  });
}

function renderHistory(history) {
  const list = $('#history-list'); list.replaceChildren();
  history.slice(0, 20).forEach(item => { const row = document.createElement('li'); const title = document.createElement('strong'); const display = item.title || sourceLabel(item.source_url); title.textContent = display; row.append(title, ` · ${item.outcome}`); if (item.title) { const button = actionButton('Artist info', `Show artist information for ${display}`, () => { button.disabled = true; loadEnrichment(item.title, value => renderHistoryEnrichment(row, value)); }); button.classList.add('history-info-button'); row.append(' ', button); } list.append(row); });
  if (!history.length) { const row = document.createElement('li'); row.textContent = 'Nothing has played yet.'; list.append(row); }
}

function renderHistoryEnrichment(row, value) { let panel = row.querySelector('.history-enrichment'); if (value.status !== 'ready') return; if (!panel) { panel = document.createElement('div'); panel.className = 'history-enrichment'; row.append(panel); } panel.replaceChildren(); if (value.image?.url) { const image = document.createElement('img'); image.src = value.image.url; image.alt = `${value.hint?.artist || 'Artist'} photo`; panel.append(image); } const strong = document.createElement('strong'); strong.textContent = value.hint?.artist || ''; const text = document.createElement('span'); text.textContent = [value.genres?.join(' · '), value.related_artists?.length ? `Related: ${value.related_artists.map(item => item.name).join(', ')}` : ''].filter(Boolean).join(' — '); panel.append(strong, document.createElement('br'), text); }

async function queueStation(station) { try { render(await api('/api/v1/queue/items', { method:'POST', body:JSON.stringify({ url:station.stream_url, display_name:state.session ? '' : 'Station shelf' }) })); showToast(`${station.name} added to the queue.`); } catch (error) { showToast(error.message); } }
async function toggleFavorite(station) { try { await api(`/api/v1/stations/${station.id}/favorite`, { method:'PUT', body:JSON.stringify({ favorite:!station.favorite }) }); await loadLibrary(); } catch (error) { showToast(error.message); } }
async function deleteStation(station) { try { await api(`/api/v1/stations/${station.id}`, { method:'DELETE' }); await loadLibrary(); } catch (error) { showToast(error.message); } }
async function deletePlaylist(playlist) { try { await api(`/api/v1/playlists/${playlist.id}`, { method:'DELETE' }); await loadLibrary(); } catch (error) { showToast(error.message); } }
async function queuePlaylist(playlist) { for (const item of playlist.items) { try { await api('/api/v1/queue/items', { method:'POST', body:JSON.stringify({ url:item.source_url }) }); } catch (error) { showToast(error.message); break; } } await refresh(); }

$('#pause-button').addEventListener('click', () => control('/api/v1/playback/pause'));
$('#resume-button').addEventListener('click', () => control('/api/v1/playback/resume'));
$('#skip-button').addEventListener('click', async () => { try { const voted = state.snapshot?.skip_vote?.voted; render(await api('/api/v1/queue/skip', { method: voted ? 'DELETE' : 'POST', headers: revisionHeader() })); } catch (error) { showToast(error.message); refresh(); } });
$('#clear-button').addEventListener('click', async () => { try { render(await api('/api/v1/queue', { method: 'DELETE', headers: revisionHeader() })); } catch (error) { showToast(error.message); } });
$('#volume').addEventListener('input', event => { $('#volume-output').value = event.target.value; });
$('#volume').addEventListener('change', async event => { try { render(await api('/api/v1/playback/volume', { method: 'PUT', body: JSON.stringify({ volume: Number(event.target.value) }) })); } catch (error) { showToast(error.message); } });
$('#autoqueue-toggle').addEventListener('click', async event => { const button = event.currentTarget; button.disabled = true; try { const enabled = button.dataset.enabled !== 'true'; await api('/api/v1/autoqueue', { method:'PUT', body:JSON.stringify({ enabled }) }); await loadAutoQueueStatus(); showToast(`Auto-queue turned ${enabled ? 'on' : 'off'}.`); } catch (error) { showToast(error.message); } finally { button.disabled = false; } });
$('#autoqueue-mode').addEventListener('change', async event => { try { await api('/api/v1/autoqueue', { method:'PUT', body:JSON.stringify({ mode:event.target.value }) }); await loadAutoQueueStatus(); showToast('Auto-queue strategy updated.'); } catch (error) { showToast(error.message); await loadAutoQueueStatus(); } });
$('#autoqueue-seeds').addEventListener('submit', async event => { event.preventDefault(); const button = event.submitter; button.disabled = true; try { await api('/api/v1/autoqueue', { method:'PUT', body:JSON.stringify({ artists:$('#autoqueue-artists').value, genres:$('#autoqueue-genres').value }) }); await loadAutoQueueStatus(); showToast('Auto-queue mix saved.'); } catch (error) { showToast(error.message); } finally { button.disabled = false; } });

$('#account-button').addEventListener('click', async () => { if (state.session) { try { await api('/api/v1/auth/logout', { method: 'POST' }); state.session = null; renderIdentity(); await loadLibrary(); showToast('Logged out. Anonymous queueing is still available.'); } catch (error) { showToast(error.message); } return; } resetAuth(); $('#auth-dialog').showModal(); $('#login-username').focus(); });
$('#auth-close').addEventListener('click', () => $('#auth-dialog').close());
$('#signup-back').addEventListener('click', resetAuth);
$('#login-form').addEventListener('submit', async event => {
  event.preventDefault(); const username = $('#login-username').value; const password = $('#login-password').value; $('#login-error').textContent = '';
  try { const result = await api('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) });
    if (result.status === 'account_creation_required') { state.pendingSignup = { username: result.username, password }; $('#signup-username').textContent = result.username; $('#login-form').hidden = true; $('#signup-form').hidden = false; $('#signup-confirmation').value = ''; $('#signup-confirmation').focus(); return; }
    state.session = result.session; $('#auth-dialog').close(); renderIdentity(); await loadLibrary(); showToast(`Welcome back, ${result.session.user.username}.`);
  } catch (error) { $('#login-error').textContent = error.message; }
});
$('#signup-form').addEventListener('submit', async event => {
  event.preventDefault(); $('#signup-error').textContent = ''; if (!state.pendingSignup) return;
  try { const result = await api('/api/v1/auth/signup', { method: 'POST', body: JSON.stringify({ username: state.pendingSignup.username, password: state.pendingSignup.password, password_confirmation: $('#signup-confirmation').value }) }); state.session = result.session; state.pendingSignup = null; $('#auth-dialog').close(); renderIdentity(); await loadLibrary(); showToast(`Account created. Welcome, ${result.session.user.username}.`); }
  catch (error) { $('#signup-error').textContent = error.message; }
});

$('#station-form').addEventListener('submit', async event => { event.preventDefault(); const form = new FormData(event.currentTarget); try { await api('/api/v1/stations', { method:'POST', body:JSON.stringify({ name:form.get('name'), stream_url:form.get('stream_url') }) }); event.currentTarget.reset(); await loadLibrary(); showToast('Station saved.'); } catch (error) { showToast(error.message); } });
$('#playlist-form').addEventListener('submit', async event => { event.preventDefault(); const form = new FormData(event.currentTarget); try { await api('/api/v1/playlists', { method:'POST', body:JSON.stringify({ name:form.get('name') }) }); event.currentTarget.reset(); await loadLibrary(); showToast('Playlist created.'); } catch (error) { showToast(error.message); } });
$('#library-search').addEventListener('input', () => { clearTimeout(loadLibrary.timer); loadLibrary.timer = setTimeout(loadLibrary, 250); });

function navigate() {
  let page = location.hash.replace('#', '') || 'home';
  if (page === 'account' && !state.session) page = 'home';
  if (page === 'admin' && !state.session?.user?.is_admin) page = 'home';
  document.querySelectorAll('.app-page').forEach(section => { section.hidden = section.dataset.page !== page; });
  document.querySelectorAll('.primary-nav a,.mobile-nav a').forEach(link => link.classList.toggle('active', link.getAttribute('href') === `#${page}`));
  if (page === 'account') loadAccount();
  if (page === 'admin') loadAdmin();
  document.querySelector('main')?.focus({ preventScroll: true });
}

async function loadAccount() {
  const root = $('#account-dashboard'); root.innerHTML = '<p class="loading-card">Loading your listening dashboard…</p>';
  try {
    const value = await api('/api/v1/account'); root.replaceChildren();
    const genres = dashboardCard('Your genres'); const genreCloud = document.createElement('div'); genreCloud.className = 'genre-cloud'; (value.genres || []).forEach(item => { const tag = document.createElement('button'); tag.type = 'button'; tag.textContent = `${item.name} · ${item.count}`; tag.addEventListener('click', () => discoverGenre(item.name)); genreCloud.append(tag); }); if (!value.genres?.length) genreCloud.textContent = 'Play some enriched tracks to build your taste profile.'; genres.append(genreCloud);
    const recent = dashboardCard('Recently played'); (value.recent || []).slice(0, 20).forEach(item => recent.append(queueableRow(item.title || sourceLabel(item.source_url), item.source_url)));
    const favorites = dashboardCard('Favorite stations'); (value.favorites || []).forEach(item => favorites.append(queueableRow(item.name, item.stream_url)));
    const playlists = dashboardCard('Playlists'); (value.playlists || []).forEach(item => { const row = document.createElement('div'); row.className = 'dashboard-row'; row.append(Object.assign(document.createElement('strong'), { textContent: item.name }), Object.assign(document.createElement('span'), { textContent: `${item.items.length} items` })); const button = actionButton('▶', `Queue ${item.name}`, () => queuePlaylist(item)); row.append(button); playlists.append(row); });
    root.append(genres, recent, favorites, playlists);
  } catch (error) { root.textContent = error.message; }
}
function dashboardCard(title) { const card = document.createElement('article'); card.className = 'dashboard-card'; const heading = document.createElement('h3'); heading.textContent = title; card.append(heading); return card; }
function queueableRow(label, url) { const row = document.createElement('div'); row.className = 'dashboard-row'; const text = document.createElement('span'); text.textContent = label; row.append(text, actionButton('＋', `Queue ${label}`, () => queueURL(url, label))); return row; }
async function queueURL(url, label = 'Item', storedTitle = label) { try { render(await api('/api/v1/queue/items', { method:'POST', body:JSON.stringify({ url, title:storedTitle === 'Item' || storedTitle === 'Stream' ? '' : storedTitle }) })); showToast(`${label} added to the queue.`); } catch (error) { showToast(error.message); } }

async function loadAdmin() {
  const root = $('#admin-settings'); root.innerHTML = '<p class="loading-card">Loading configuration…</p>';
  try {
    const [settingsBody, usersBody] = await Promise.all([api('/api/v1/admin/settings'), api('/api/v1/admin/users')]); root.replaceChildren();
    const groups = new Map(); settingsBody.settings.forEach(setting => { if (!groups.has(setting.category)) groups.set(setting.category, []); groups.get(setting.category).push(setting); });
    groups.forEach((items, category) => { const section = document.createElement('section'); section.className = 'settings-group'; const heading = document.createElement('h3'); heading.textContent = category; section.append(heading); items.forEach(setting => section.append(renderSetting(setting))); root.append(section); });
    const link = document.createElement('a'); link.href = settingsBody.links.lastfm_create_key; link.target = '_blank'; link.rel = 'noopener noreferrer'; link.textContent = 'Create a Last.fm API key ↗'; root.querySelector('.settings-group:nth-child(3)')?.append(link);
    renderAdminUsers(usersBody.users || []);
  } catch (error) { root.textContent = error.message; }
}
function renderSetting(setting) {
  const form = document.createElement('form'); form.className = 'setting-row'; const copy = document.createElement('div'); const label = document.createElement('strong'); label.textContent = setting.label; const description = document.createElement('small'); description.textContent = setting.description + (setting.restart_required ? ' Applies after service restart.' : ''); copy.append(label, description);
  let input; if (setting.type === 'select') { input = document.createElement('select'); setting.options.forEach(value => { const option = document.createElement('option'); option.value = value; option.textContent = value.replaceAll('_', ' '); option.selected = value === setting.value; input.append(option); }); } else if (setting.type === 'boolean') { input = document.createElement('select'); [['true','On'],['false','Off']].forEach(([value,text]) => { const option = new Option(text,value,value === setting.value,value === setting.value); input.append(option); }); } else { input = document.createElement('input'); input.type = setting.secret ? 'password' : setting.type === 'number' ? 'number' : 'text'; input.value = setting.secret ? '' : setting.value || ''; input.placeholder = setting.secret && setting.configured ? 'Configured — enter to replace' : ''; }
  if (setting.read_only) input.disabled = true;
  input.setAttribute('aria-label', setting.label); const save = document.createElement('button'); save.className = 'button button-primary'; save.type = 'submit'; save.textContent = 'Save'; if (setting.read_only) save.hidden = true; form.append(copy, input, save); form.addEventListener('submit', async event => { event.preventDefault(); if (setting.read_only) return; if (setting.secret && !input.value) return showToast('Enter a new value, or use Remove.'); try { await api(`/api/v1/admin/settings/${setting.key}`, { method:'PUT', body:JSON.stringify({ value:input.value }) }); input.value = setting.secret ? '' : input.value; showToast(`${setting.label} saved.`); } catch (error) { showToast(error.message); } });
  if (setting.secret && setting.configured) { const remove = document.createElement('button'); remove.className = 'button button-quiet dark-text'; remove.type = 'button'; remove.textContent = 'Remove'; remove.addEventListener('click', async () => { try { await api(`/api/v1/admin/settings/${setting.key}`, { method:'DELETE' }); showToast(`${setting.label} removed.`); loadAdmin(); } catch (error) { showToast(error.message); } }); form.append(remove); }
  if (setting.key === 'lastfm_api_key') { const test = document.createElement('button'); test.className = 'button button-quiet dark-text'; test.type = 'button'; test.textContent = 'Test'; test.addEventListener('click', async () => { try { await api('/api/v1/admin/lastfm/test', { method:'POST', body:JSON.stringify({ api_key:input.value }) }); showToast('Last.fm connection succeeded.'); } catch (error) { showToast(error.message); } }); form.append(test); }
  return form;
}
function renderAdminUsers(users) { const root = $('#admin-user-list'); root.replaceChildren(); users.forEach(user => { const row = document.createElement('div'); row.className = 'user-role-row'; const copy = document.createElement('span'); copy.textContent = user.username; const button = document.createElement('button'); button.className = 'button button-quiet dark-text'; button.textContent = user.is_admin ? 'Remove admin' : 'Make admin'; button.addEventListener('click', async () => { try { await api(`/api/v1/admin/users/${user.id}/role`, { method:'PUT', body:JSON.stringify({ admin:!user.is_admin }) }); loadAdmin(); } catch (error) { showToast(error.message); } }); row.append(copy, button); root.append(row); }); }

async function discover(query) { if (!query) return; location.hash = '#discover'; navigate(); $('#youtube-query').value = query; await runDiscovery(query); }
async function discoverGenre(genre) { if (!genre) return; location.hash = '#discover'; navigate(); $('#youtube-query').value = genre; const local = $('#local-discovery'); const videos = $('#youtube-results'); local.replaceChildren(); videos.replaceChildren(); $('#youtube-status').textContent = `Loading top ${genre} artists and tracks from Last.fm…`; try { const body = await api(`/api/v1/discovery?genre=${encodeURIComponent(genre)}`); $('#youtube-status').textContent = `Top artists and tracks tagged ${body.genre} on Last.fm`; const columns = document.createElement('div'); columns.className = 'genre-results'; columns.append(discoveryList('Artists', body.artists, item => item.name, item => item.name), discoveryList('Tracks', body.tracks, item => `${item.artist} — ${item.name}`, item => `${item.artist} ${item.name}`)); local.append(columns); } catch (error) { $('#youtube-status').textContent = error.message; } }
function discoveryList(title, items, labelFor, queryFor) { const section = document.createElement('section'); section.className = 'discovery-list'; const heading = document.createElement('h3'); heading.textContent = title; section.append(heading); const list = document.createElement('ol'); (items || []).forEach(item => { const row = document.createElement('li'); const source = document.createElement('a'); source.href = item.url || item.artist_url || '#'; source.target = '_blank'; source.rel = 'noopener noreferrer'; source.textContent = labelFor(item); const youtube = document.createElement('a'); youtube.href = '#discover'; youtube.className = 'youtube-search-link'; youtube.textContent = 'Search YouTube'; youtube.addEventListener('click', event => { event.preventDefault(); discover(queryFor(item)); }); row.append(source, youtube); list.append(row); }); if (!items?.length) { const empty = document.createElement('li'); empty.textContent = 'No results returned.'; list.append(empty); } section.append(list); return section; }
async function runDiscovery(query) {
  const results = $('#youtube-results'); const local = $('#local-discovery'); $('#youtube-status').textContent = `Discovering ${query}…`; results.replaceChildren(); local.replaceChildren();
  const [known, youtube] = await Promise.allSettled([api(`/api/v1/discovery?q=${encodeURIComponent(query)}`), api(`/api/v1/youtube/search?q=${encodeURIComponent(query)}`)]);
  if (known.status === 'fulfilled' && known.value.matches?.length) { const heading = document.createElement('h3'); heading.textContent = 'Known around the house'; local.append(heading); const grid = document.createElement('div'); grid.className = 'discovery-chips'; known.value.matches.forEach(item => { const button = document.createElement('button'); button.type = 'button'; button.className = 'discovery-chip'; button.textContent = [item.hint?.artist,item.hint?.title].filter(Boolean).join(' — '); button.addEventListener('click', () => discover(button.textContent)); grid.append(button); }); local.append(grid); }
  if (youtube.status === 'fulfilled') { const body = youtube.value; $('#youtube-status').textContent = `${body.results.length} playable result${body.results.length === 1 ? '' : 's'} for ${query}`; body.results.forEach(item => { const card = document.createElement('article'); card.className = 'youtube-card'; if (item.thumbnail) { const image = document.createElement('img'); image.src = item.thumbnail; image.alt = ''; card.append(image); } const copy = document.createElement('div'); const title = document.createElement('strong'); title.textContent = item.title; const meta = document.createElement('small'); meta.textContent = [item.channel, item.duration_seconds ? formatTime(item.duration_seconds) : ''].filter(Boolean).join(' · '); copy.append(title, meta); const button = document.createElement('button'); button.className = 'button button-primary'; button.textContent = 'Queue'; button.addEventListener('click', () => queueURL(item.url, item.title, item.title.includes(' - ') || !item.channel ? item.title : `${item.channel} - ${item.title}`)); card.append(copy, button); results.append(card); }); } else { $('#youtube-status').textContent = youtube.reason.message; }
}
$('#youtube-search-form').addEventListener('submit', async event => {
  event.preventDefault(); const query = $('#youtube-query').value.trim();
  if (/^https?:\/\//i.test(query)) { await queueURL(query, 'Stream'); $('#youtube-query').value = ''; return; }
  runDiscovery(query);
});

function renderSetupStep() { const steps = document.querySelectorAll('.setup-step'); steps.forEach((step, index) => { step.hidden = index !== state.setupStep; }); $('#setup-step-label').textContent = `Step ${state.setupStep + 1} of ${steps.length}`; $('#setup-back').hidden = state.setupStep === 0; $('#setup-next').hidden = state.setupStep === steps.length - 1; $('#setup-submit').hidden = state.setupStep !== steps.length - 1; $('#setup-next').textContent = state.setupStep === 0 ? 'Begin setup' : 'Continue'; if (state.setupStep === steps.length - 1) { const form = new FormData($('#setup-form')); $('#setup-review').textContent = `Administrator: ${form.get('username')} · Access: ${String(form.get('access_mode')).replaceAll('_',' ')} · Last.fm: ${form.get('lastfm_api_key') ? 'configured' : 'not configured'}`; } steps[state.setupStep].querySelector('input')?.focus(); }
$('#setup-next').addEventListener('click', () => { if (state.setupStep === 2) { const inputs = document.querySelectorAll('[data-setup-step="2"] input'); if (![...inputs].every(input => input.reportValidity())) return; if (inputs[1].value !== inputs[2].value) return showToast('Passwords do not match.'); } state.setupStep++; renderSetupStep(); });
$('#setup-back').addEventListener('click', () => { state.setupStep--; renderSetupStep(); });
$('#setup-form').addEventListener('submit', async event => { event.preventDefault(); const form = new FormData(event.currentTarget); $('#setup-error').textContent = ''; try { const result = await api('/api/v1/setup/complete', { method:'POST', body:JSON.stringify(Object.fromEntries(form)) }); state.session = result.session; $('#setup-shell').hidden = true; $('#app-shell').hidden = false; renderIdentity(); await Promise.all([refresh(), loadLibrary()]); connectEvents(); location.hash = '#home'; navigate(); showToast('Your house jukebox is ready.'); } catch (error) { $('#setup-error').textContent = error.message; } });

window.addEventListener('hashchange', navigate);
async function boot() { try { const setup = await api('/api/v1/setup/status'); if (!setup.installed) { $('#app-shell').hidden = true; $('#setup-shell').hidden = false; renderSetupStep(); return; } } catch (error) { showToast(error.message); } await Promise.all([refresh(), loadSession(), loadAutoQueueStatus()]); connectEvents(); navigate(); }
boot();
setInterval(() => { refresh(); loadAutoQueueStatus(); }, 30000);
