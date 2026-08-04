const state = { snapshot: null, session: null, pendingSignup: null, source: null };
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
  $('#now-playing-heading').textContent = playback.title || (current ? sourceLabel(current.source.url) : 'Nothing playing');
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
  renderQueue(snapshot.items);
}

function sourceLabel(url) {
  try { const parsed = new URL(url); return parsed.pathname.split('/').filter(Boolean).pop() || parsed.hostname; } catch { return url; }
}

function renderQueue(items) {
  const list = $('#queue-list'); list.replaceChildren(); $('#queue-empty').hidden = items.length > 0; $('#clear-button').hidden = items.length === 0;
  items.forEach((item, index) => {
    const row = document.createElement('li'); row.className = `queue-item ${item.status}`;
    const number = document.createElement('span'); number.className = 'queue-number'; number.textContent = item.status === 'current' ? '▶' : String(index + 1).padStart(2, '0');
    const copy = document.createElement('div'); copy.className = 'queue-copy';
    const title = document.createElement('span'); title.className = 'queue-title'; title.textContent = sourceLabel(item.source.url); title.title = item.source.url;
    const meta = document.createElement('div'); meta.className = 'queue-meta';
    const submitter = item.submitter.kind === 'user' ? item.submitter.username : item.submitter.display_name || 'Anonymous';
    meta.textContent = item.error ? `${submitter} · ${item.error}` : `Added by ${submitter}`; copy.append(title, meta);
    const badge = document.createElement('span'); badge.className = 'queue-badge'; badge.textContent = item.status;
    const actions = document.createElement('div'); actions.className = 'item-actions';
    if (index > 0) actions.append(actionButton('↑', `Move ${title.textContent} up`, () => moveItem(index, -1)));
    if (index < items.length - 1) actions.append(actionButton('↓', `Move ${title.textContent} down`, () => moveItem(index, 1)));
    actions.append(actionButton('×', `Remove ${title.textContent}`, () => removeItem(item.id)));
    row.append(number, copy, badge, actions); list.append(row);
  });
}

function actionButton(text, label, handler) { const button = document.createElement('button'); button.type = 'button'; button.className = 'icon-button'; button.textContent = text; button.setAttribute('aria-label', label); button.addEventListener('click', handler); return button; }
function revisionHeader() { return { 'If-Match': `"${state.snapshot?.revision ?? 0}"` }; }
async function removeItem(id) { try { render(await api(`/api/v1/queue/items/${id}`, { method: 'DELETE', headers: revisionHeader() })); } catch (error) { showToast(error.message); refresh(); } }
async function moveItem(index, delta) { const ids = state.snapshot.items.map(item => item.id); [ids[index], ids[index + delta]] = [ids[index + delta], ids[index]]; try { render(await api('/api/v1/queue/order', { method: 'PUT', headers: revisionHeader(), body: JSON.stringify({ item_ids: ids }) })); } catch (error) { showToast(error.message); refresh(); } }
async function control(path, options = {}) { try { render(await api(path, { method: 'POST', ...options })); } catch (error) { showToast(error.message); } }
async function refresh() { try { render(await api('/api/v1/queue')); } catch (error) { showToast(error.message); } }

function connectEvents() {
  state.source?.close(); const connection = $('#connection-status'); state.source = new EventSource('/api/v1/events');
  state.source.addEventListener('open', () => { connection.className = 'connection online'; connection.lastChild.textContent = ' Connected'; });
  state.source.addEventListener('queue', event => { try { render(JSON.parse(event.data)); } catch { showToast('Received an invalid player update.'); } });
  state.source.addEventListener('error', () => { connection.className = 'connection offline'; connection.lastChild.textContent = ' Reconnecting'; });
}

async function loadSession() {
  try { const result = await api('/api/v1/auth/session'); state.session = result.authenticated ? result.session : null; renderIdentity(); } catch { state.session = null; renderIdentity(); }
}

function renderIdentity() {
  const button = $('#account-button'); const footer = $('#footer-identity');
  if (state.session) { button.textContent = `${state.session.user.username} · Log out`; footer.textContent = `Signed in as ${state.session.user.username}`; }
  else { button.textContent = 'Log in'; footer.textContent = 'Browsing anonymously'; }
}

function resetAuth() { $('#login-form').hidden = false; $('#signup-form').hidden = true; $('#login-error').textContent = ''; $('#signup-error').textContent = ''; state.pendingSignup = null; }

$('#add-form').addEventListener('submit', async event => {
  event.preventDefault(); const form = new FormData(event.currentTarget);
  try { const snapshot = await api('/api/v1/queue/items', { method: 'POST', body: JSON.stringify({ url: form.get('url'), display_name: form.get('display_name') }) }); render(snapshot); $('#stream-url').value = ''; showToast('Added to the house queue.'); }
  catch (error) { showToast(error.message); }
});
$('#pause-button').addEventListener('click', () => control('/api/v1/playback/pause'));
$('#resume-button').addEventListener('click', () => control('/api/v1/playback/resume'));
$('#skip-button').addEventListener('click', () => control('/api/v1/queue/skip', { headers: revisionHeader() }));
$('#clear-button').addEventListener('click', async () => { try { render(await api('/api/v1/queue', { method: 'DELETE', headers: revisionHeader() })); } catch (error) { showToast(error.message); } });
$('#volume').addEventListener('input', event => { $('#volume-output').value = event.target.value; });
$('#volume').addEventListener('change', async event => { try { render(await api('/api/v1/playback/volume', { method: 'PUT', body: JSON.stringify({ volume: Number(event.target.value) }) })); } catch (error) { showToast(error.message); } });

$('#account-button').addEventListener('click', async () => { if (state.session) { try { await api('/api/v1/auth/logout', { method: 'POST' }); state.session = null; renderIdentity(); showToast('Logged out. Anonymous queueing is still available.'); } catch (error) { showToast(error.message); } return; } resetAuth(); $('#auth-dialog').showModal(); $('#login-username').focus(); });
$('#auth-close').addEventListener('click', () => $('#auth-dialog').close());
$('#signup-back').addEventListener('click', resetAuth);
$('#login-form').addEventListener('submit', async event => {
  event.preventDefault(); const username = $('#login-username').value; const password = $('#login-password').value; $('#login-error').textContent = '';
  try { const result = await api('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) });
    if (result.status === 'account_creation_required') { state.pendingSignup = { username: result.username, password }; $('#signup-username').textContent = result.username; $('#login-form').hidden = true; $('#signup-form').hidden = false; $('#signup-confirmation').value = ''; $('#signup-confirmation').focus(); return; }
    state.session = result.session; $('#auth-dialog').close(); renderIdentity(); showToast(`Welcome back, ${result.session.user.username}.`);
  } catch (error) { $('#login-error').textContent = error.message; }
});
$('#signup-form').addEventListener('submit', async event => {
  event.preventDefault(); $('#signup-error').textContent = ''; if (!state.pendingSignup) return;
  try { const result = await api('/api/v1/auth/signup', { method: 'POST', body: JSON.stringify({ username: state.pendingSignup.username, password: state.pendingSignup.password, password_confirmation: $('#signup-confirmation').value }) }); state.session = result.session; state.pendingSignup = null; $('#auth-dialog').close(); renderIdentity(); showToast(`Account created. Welcome, ${result.session.user.username}.`); }
  catch (error) { $('#signup-error').textContent = error.message; }
});

refresh(); loadSession(); connectEvents();
