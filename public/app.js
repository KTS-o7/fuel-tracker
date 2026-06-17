'use strict';

const TOKEN_KEY = 'fuel_token';

const state = {
  entries: [],
  sortBy: 'date',
  sortDir: 'desc',
  currentMonth: monthString(new Date()),
  editingId: null,
};

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

// ===== API =====

async function api(path, opts = {}) {
  const token = localStorage.getItem(TOKEN_KEY);
  const headers = Object.assign(
    { 'Content-Type': 'application/json' },
    opts.headers || {},
  );
  if (token) headers['Authorization'] = 'Bearer ' + token;

  const res = await fetch(path, Object.assign({}, opts, { headers }));
  if (res.status === 401) {
    localStorage.removeItem(TOKEN_KEY);
    showLogin();
    throw new Error('unauthorized');
  }
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const j = await res.json();
      if (j.error) msg = j.error;
    } catch (_) {}
    throw new Error(msg);
  }
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return res;
}

// ===== Auth =====

function showLogin() {
  $('#login-view').classList.remove('hidden');
  $('#app-view').classList.add('hidden');
  $('#login-form').reset();
  $('#login-error').textContent = '';
  setTimeout(() => $('input[name="username"]', $('#login-form')).focus(), 50);
}

function showApp() {
  $('#login-view').classList.add('hidden');
  $('#app-view').classList.remove('hidden');
}

async function handleLogin(e) {
  e.preventDefault();
  const fd = new FormData(e.target);
  const username = fd.get('username');
  const password = fd.get('password');
  $('#login-error').textContent = '';
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (!res.ok) {
      const j = await res.json().catch(() => ({}));
      $('#login-error').textContent = j.error || 'Sign in failed';
      return;
    }
    const { token } = await res.json();
    localStorage.setItem(TOKEN_KEY, token);
    showApp();
    initApp();
  } catch (err) {
    $('#login-error').textContent = 'Network error';
  }
}

function handleLogout() {
  localStorage.removeItem(TOKEN_KEY);
  showLogin();
}

// ===== Tabs =====

function activateTab(name) {
  $$('.tab').forEach((t) => t.classList.toggle('active', t.dataset.tab === name));
  $$('.panel').forEach((p) => p.classList.toggle('hidden', p.dataset.panel !== name));
  if (name === 'history') renderHistory();
  if (name === 'dashboard') renderDashboard();
}

// ===== Add form =====

function defaultAddForm() {
  const form = $('#add-form');
  form.reset();
  const today = new Date().toISOString().slice(0, 10);
  $('input[name="date"]', form).value = today;
  $('input[name="price_per_l"]', form).value = '110.89';
  $('input[name="fuel_type"][value="regular"]', form).checked = true;
  $('#add-message').textContent = '';
  $('#add-message').classList.remove('error');
}

async function handleAdd(e) {
  e.preventDefault();
  const form = e.target;
  const msg = $('#add-message');
  msg.textContent = '';
  msg.classList.remove('error');
  const body = formToBody(form);
  try {
    await api('/api/entries', { method: 'POST', body: JSON.stringify(body) });
    msg.textContent = 'Saved';
    msg.classList.add('success');
    form.reset();
    $('input[name="price_per_l"]', form).value = '110.89';
    $('input[name="fuel_type"][value="regular"]', form).checked = true;
    $('input[name="date"]', form).value = new Date().toISOString().slice(0, 10);
    toast('Entry saved');
    await loadEntries();
    setTimeout(() => activateTab('history'), 350);
  } catch (err) {
    msg.textContent = err.message;
    msg.classList.add('error');
  }
}

function formToBody(form) {
  const fd = new FormData(form);
  return {
    date: fd.get('date'),
    odometer: parseFloat(fd.get('odometer')),
    liters: parseFloat(fd.get('liters')),
    price_per_l: parseFloat(fd.get('price_per_l')),
    fuel_type: fd.get('fuel_type') || 'regular',
    notes: fd.get('notes') || '',
  };
}

// ===== History =====

async function loadEntries() {
  state.entries = await api('/api/entries');
}

function sortEntries() {
  const dir = state.sortDir === 'asc' ? 1 : -1;
  const k = state.sortBy;
  return [...state.entries].sort((a, b) => {
    const va = a[k];
    const vb = b[k];
    if (va == null && vb == null) return 0;
    if (va == null) return 1;
    if (vb == null) return -1;
    if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * dir;
    return String(va).localeCompare(String(vb)) * dir;
  });
}

function renderHistory() {
  const body = $('#history-body');
  const empty = $('#history-empty');
  if (state.entries.length === 0) {
    body.innerHTML = '';
    empty.classList.remove('hidden');
    return;
  }
  empty.classList.add('hidden');
  const sorted = sortEntries();
  body.innerHTML = sorted
    .map((e) => {
      const kmpl = e.kmpl == null ? '<span class="kmpl-na">—</span>' : e.kmpl.toFixed(2);
      const fuel = `<span class="fuel-badge ${escapeAttr(e.fuel_type)}">${escapeHTML(e.fuel_type)}</span>`;
      const notes = e.notes ? escapeHTML(e.notes) : '';
      return `
        <tr data-id="${e.id}">
          <td>${escapeHTML(e.date)}</td>
          <td class="num">${e.odometer.toFixed(2)}</td>
          <td class="num">${e.liters.toFixed(2)}</td>
          <td class="num">${e.price_per_l.toFixed(2)}</td>
          <td class="num">${e.total_cost.toFixed(2)}</td>
          <td>${fuel}</td>
          <td class="num">${kmpl}</td>
          <td class="notes">${notes}</td>
          <td class="row-actions"><button class="edit">Edit</button></td>
        </tr>`;
    })
    .join('');

  $$('th[data-sort]', $('#history-table')).forEach((th) => {
    th.classList.toggle('sorted-asc', state.sortBy === th.dataset.sort && state.sortDir === 'asc');
    th.classList.toggle('sorted-desc', state.sortBy === th.dataset.sort && state.sortDir === 'desc');
  });

  $$('tr[data-id]', body).forEach((tr) => {
    const id = parseInt(tr.dataset.id, 10);
    tr.addEventListener('click', () => openEdit(id));
  });
}

function handleSort(e) {
  const th = e.target.closest('th[data-sort]');
  if (!th) return;
  const key = th.dataset.sort;
  if (state.sortBy === key) {
    state.sortDir = state.sortDir === 'asc' ? 'desc' : 'asc';
  } else {
    state.sortBy = key;
    state.sortDir = ['odometer', 'liters', 'price_per_l', 'total_cost', 'kmpl'].includes(key) ? 'desc' : 'asc';
  }
  renderHistory();
}

// ===== Edit modal =====

function openEdit(id) {
  const entry = state.entries.find((e) => e.id === id);
  if (!entry) return;
  state.editingId = id;
  const form = $('#edit-form');
  form.elements['date'].value = entry.date;
  form.elements['odometer'].value = entry.odometer;
  form.elements['liters'].value = entry.liters;
  form.elements['price_per_l'].value = entry.price_per_l;
  form.elements['notes'].value = entry.notes || '';
  const fuel = entry.fuel_type || 'regular';
  const radio = form.querySelector(`input[name="fuel_type"][value="${fuel}"]`);
  if (radio) radio.checked = true;
  $('#modal-overlay').classList.remove('hidden');
}

function closeEdit() {
  state.editingId = null;
  $('#modal-overlay').classList.add('hidden');
}

async function handleEdit(e) {
  e.preventDefault();
  const body = formToBody(e.target);
  try {
    await api(`/api/entries/${state.editingId}`, { method: 'PUT', body: JSON.stringify(body) });
    toast('Updated');
    closeEdit();
    await loadEntries();
    renderHistory();
  } catch (err) {
    toast(err.message, 'error');
  }
}

async function handleDelete() {
  if (!state.editingId) return;
  if (!confirm('Delete this entry?')) return;
  try {
    await api(`/api/entries/${state.editingId}`, { method: 'DELETE' });
    toast('Deleted');
    closeEdit();
    await loadEntries();
    renderHistory();
  } catch (err) {
    toast(err.message, 'error');
  }
}

// ===== Export =====

function handleExport(e) {
  const kind = e.target.dataset.export;
  if (!kind) return;
  if (kind === 'lifetime') downloadCSV('2000-01-01', '2999-12-31');
  else if (kind === 'month') {
    const [y, m] = state.currentMonth.split('-');
    const last = new Date(parseInt(y, 10), parseInt(m, 10), 0).toISOString().slice(0, 10);
    downloadCSV(`${state.currentMonth}-01`, last);
  } else if (kind === 'range') {
    const form = $('#range-form');
    const today = new Date().toISOString().slice(0, 10);
    const firstOfMonth = new Date().toISOString().slice(0, 8) + '01';
    form.elements['from'].value = firstOfMonth;
    form.elements['to'].value = today;
    $('#range-overlay').classList.remove('hidden');
  }
}

async function handleRangeSubmit(e) {
  e.preventDefault();
  const fd = new FormData(e.target);
  $('#range-overlay').classList.add('hidden');
  downloadCSV(fd.get('from'), fd.get('to'));
}

async function downloadCSV(from, to) {
  const token = localStorage.getItem(TOKEN_KEY);
  const res = await fetch(`/api/export?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`, {
    headers: { Authorization: 'Bearer ' + token },
  });
  if (!res.ok) {
    toast('Export failed', 'error');
    return;
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `fuel-${from}-to-${to}.csv`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
  toast('CSV downloaded', 'success');
}

// ===== Dashboard =====

function shiftMonth(delta) {
  const [y, m] = state.currentMonth.split('-').map(Number);
  const d = new Date(y, m - 1 + delta, 1);
  state.currentMonth = monthString(d);
  renderDashboard();
}

async function renderDashboard() {
  const [y, m] = state.currentMonth.split('-');
  const label = new Date(y, m - 1, 1).toLocaleDateString(undefined, {
    month: 'long',
    year: 'numeric',
  });
  $('#month-label').textContent = label;

  try {
    const stats = await api(`/api/stats?month=${state.currentMonth}`);
    $('#stat-km').textContent = stats.total_km ? stats.total_km.toFixed(1) : '—';
    $('#stat-cost').textContent = stats.total_cost ? '₹' + stats.total_cost.toFixed(0) : '—';
    $('#stat-liters').textContent = stats.total_liters ? stats.total_liters.toFixed(1) : '—';
    $('#stat-kmpl').textContent = stats.avg_kmpl ? stats.avg_kmpl.toFixed(2) : '—';
    $('#stat-count').textContent = stats.fill_count || '—';

    const monthEntries = (await api(`/api/entries?month=${state.currentMonth}`))
      .slice()
      .reverse();
    drawKmplChart(monthEntries);
  } catch (err) {
    // dashboard rendering failure already shown via toast
  }
}

function drawKmplChart(entries) {
  const canvas = $('#kmpl-chart');
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const cssW = canvas.clientWidth || canvas.width;
  const cssH = 240;
  canvas.width = cssW * dpr;
  canvas.height = cssH * dpr;
  canvas.style.height = cssH + 'px';
  ctx.scale(dpr, dpr);
  ctx.clearRect(0, 0, cssW, cssH);

  const placeholder = $('#chart-placeholder');
  const points = entries.filter((e) => e.kmpl != null);
  placeholder.classList.toggle('hidden', points.length > 0);
  if (points.length === 0) return;

  const pad = { l: 44, r: 16, t: 16, b: 28 };
  const w = cssW - pad.l - pad.r;
  const h = cssH - pad.t - pad.b;

  const minY = Math.min(...points.map((p) => p.kmpl)) * 0.9;
  const maxY = Math.max(...points.map((p) => p.kmpl)) * 1.1;
  const yRange = Math.max(1, maxY - minY);

  // grid lines + y labels
  ctx.strokeStyle = '#e5e7eb';
  ctx.fillStyle = '#9ca3af';
  ctx.font = '11px -apple-system, sans-serif';
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (let i = 0; i <= 4; i++) {
    const y = pad.t + (h / 4) * i;
    const val = maxY - (yRange / 4) * i;
    ctx.beginPath();
    ctx.moveTo(pad.l, y);
    ctx.lineTo(cssW - pad.r, y);
    ctx.stroke();
    ctx.fillText(val.toFixed(1), pad.l - 8, y);
  }

  const xStep = points.length > 1 ? w / (points.length - 1) : 0;

  // x labels
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillStyle = '#9ca3af';
  points.forEach((p, i) => {
    const x = pad.l + xStep * i;
    const lbl = p.date.slice(5);
    ctx.fillText(lbl, x, cssH - pad.b + 6);
  });

  // line
  ctx.strokeStyle = '#2563eb';
  ctx.lineWidth = 2;
  ctx.beginPath();
  points.forEach((p, i) => {
    const x = pad.l + xStep * i;
    const y = pad.t + h - ((p.kmpl - minY) / yRange) * h;
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  ctx.stroke();

  // points
  ctx.fillStyle = '#2563eb';
  points.forEach((p, i) => {
    const x = pad.l + xStep * i;
    const y = pad.t + h - ((p.kmpl - minY) / yRange) * h;
    ctx.beginPath();
    ctx.arc(x, y, 3.5, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = '#ffffff';
    ctx.beginPath();
    ctx.arc(x, y, 1.5, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = '#2563eb';
  });
}

// ===== Toast =====

let toastTimer = null;
function toast(msg, kind = '') {
  const el = $('#toast');
  el.textContent = msg;
  el.className = 'toast ' + kind;
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.add('hidden'), 2200);
}

// ===== Utils =====

function monthString(d) {
  return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0');
}

function escapeHTML(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}
function escapeAttr(s) {
  return escapeHTML(s);
}

// ===== Init =====

async function initApp() {
  defaultAddForm();
  try {
    await loadEntries();
  } catch (err) {
    // already redirected to login if 401
  }
  activateTab('add');
}

function bindEvents() {
  $('#login-form').addEventListener('submit', handleLogin);
  $('#logout-btn').addEventListener('click', handleLogout);

  $$('.tab').forEach((t) => t.addEventListener('click', () => activateTab(t.dataset.tab)));
  $('#add-form').addEventListener('submit', handleAdd);

  $$('th[data-sort]', $('#history-table')).forEach((th) =>
    th.addEventListener('click', handleSort),
  );

  $('#modal-close').addEventListener('click', closeEdit);
  $('#cancel-btn').addEventListener('click', closeEdit);
  $('#delete-btn').addEventListener('click', handleDelete);
  $('#edit-form').addEventListener('submit', handleEdit);

  $('#range-close').addEventListener('click', () => $('#range-overlay').classList.add('hidden'));
  $('#range-cancel').addEventListener('click', () => $('#range-overlay').classList.add('hidden'));
  $('#range-form').addEventListener('submit', handleRangeSubmit);

  $$('[data-export]').forEach((b) => b.addEventListener('click', handleExport));

  $('#prev-month').addEventListener('click', () => shiftMonth(-1));
  $('#next-month').addEventListener('click', () => shiftMonth(1));

  $('#modal-overlay').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) closeEdit();
  });
  $('#range-overlay').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) e.currentTarget.classList.add('hidden');
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      if (!$('#modal-overlay').classList.contains('hidden')) closeEdit();
      if (!$('#range-overlay').classList.contains('hidden'))
        $('#range-overlay').classList.add('hidden');
    }
  });

  window.addEventListener('resize', () => {
    if (!$('#app-view').classList.contains('hidden')) {
      const active = $('.tab.active');
      if (active && active.dataset.tab === 'dashboard') renderDashboard();
    }
  });
}

bindEvents();
if (localStorage.getItem(TOKEN_KEY)) {
  showApp();
  initApp();
} else {
  showLogin();
}