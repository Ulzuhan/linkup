/* LinkUp — the script.
 *
 * Everything here is an enhancement: every form posts on its own, the
 * account menu is a <details>, the extra options are a <details>. What the
 * script adds is the live strip preview, the sliding thumb on the segmented
 * controls, the dialogs, the toasts, relative times, the search box, and
 * the calls to the JSON API for the actions that have no form (edit,
 * delete, folders, keys, domains, webhooks, CSV).
 *
 * Nothing here talks to anyone but this origin, and nothing here reads
 * anything about a visitor.
 */
(() => {
  'use strict';

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
  const on = (el, type, fn, opts) => { if (el) el.addEventListener(type, fn, opts); };

  const el = (tag, cls, text) => {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  };

  // A fresh <svg><use> for something the script builds. The symbols live in
  // the layout.
  const icon = (name) => {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('aria-hidden', 'true');
    const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
    use.setAttribute('href', '#' + name);
    svg.appendChild(use);
    return svg;
  };

  /* ── Toasts ─────────────────────────────────────────────────────────── */
  function toast(title, kind = 'info', description) {
    const host = $('#toasts');
    if (!host) return;
    const t = el('div', 'toast');
    t.dataset.kind = kind;
    t.setAttribute('role', 'status');
    const ic = el('span', 'toast-icon');
    ic.setAttribute('aria-hidden', 'true');
    ic.appendChild(icon(kind === 'success' ? 'i-circle-check' : kind === 'error' ? 'i-circle-x' : 'i-info'));
    const body = el('div', 'toast-body');
    body.appendChild(el('strong', null, title));
    if (description) body.appendChild(el('span', null, description));
    const close = el('button', 'btn btn-ghost btn-icon-sm');
    close.type = 'button';
    close.setAttribute('aria-label', 'Dismiss');
    close.appendChild(icon('i-x'));
    t.append(ic, body, close);
    host.appendChild(t);
    const remove = () => { t.classList.add('is-out'); setTimeout(() => t.remove(), 220); };
    const timer = setTimeout(remove, kind === 'error' ? 8000 : 4000);
    on(close, 'click', () => { clearTimeout(timer); remove(); });
  }

  /* ── Dialogs ────────────────────────────────────────────────────────── */
  const supportsDialog = typeof HTMLDialogElement === 'function' && 'showModal' in HTMLDialogElement.prototype;
  const openDialog = (d) => { if (!d) return; if (supportsDialog) { if (!d.open) d.showModal(); } else d.setAttribute('open', ''); };
  const closeDialog = (d) => { if (!d) return; if (supportsDialog) { if (d.open) d.close(); } else d.removeAttribute('open'); };
  function wireDialog(d) {
    if (!d) return;
    $$('[data-close]', d).forEach((b) => on(b, 'click', () => closeDialog(d)));
    // A click on the backdrop lands on the dialog itself; one on the panel
    // lands on a child.
    on(d, 'click', (e) => { if (e.target === d) closeDialog(d); });
  }

  // One question, one answer. Resolves true only for the confirming button.
  function confirmDialog({ title = 'Are you sure?', message = '', confirmLabel = 'Delete', danger = true } = {}) {
    const d = $('#confirm-dialog');
    if (!d || !supportsDialog) return Promise.resolve(window.confirm(`${title}\n\n${message}`));
    return new Promise((resolve) => {
      $('#confirm-title', d).textContent = title;
      $('#confirm-message', d).textContent = message;
      const ok = $('#confirm-ok', d);
      const cancel = $('#confirm-cancel', d);
      ok.textContent = confirmLabel;
      ok.className = danger ? 'btn btn-danger' : 'btn btn-primary';
      let answered = false;
      const cleanup = () => {
        ok.removeEventListener('click', onOk);
        cancel.removeEventListener('click', onCancel);
        d.removeEventListener('close', onClose);
      };
      const finish = (value) => { if (answered) return; answered = true; cleanup(); resolve(value); };
      const onOk = () => { finish(true); closeDialog(d); };
      const onCancel = () => { finish(false); closeDialog(d); };
      const onClose = () => finish(false);
      ok.addEventListener('click', onOk);
      cancel.addEventListener('click', onCancel);
      d.addEventListener('close', onClose);
      openDialog(d);
      cancel.focus();
    });
  }

  /* ── Fetch ──────────────────────────────────────────────────────────── */
  async function api(path, { method = 'GET', body, form } = {}) {
    const opts = { method, headers: {}, credentials: 'same-origin' };
    if (form) opts.body = form;
    else if (body !== undefined) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
    let res;
    try { res = await fetch(path, opts); }
    catch (e) { throw new Error('Network error. Check the connection and try again.'); }
    let data = null;
    try { data = await res.json(); } catch (e) { /* no body */ }
    if (!res.ok) throw new Error((data && data.error) || `The server answered ${res.status}.`);
    return data;
  }

  // Back to the page with the news in the address, the way the forms do it.
  function reloadWith(success, path) {
    const u = new URL(path || (location.pathname + location.search), location.origin);
    u.searchParams.delete('error');
    if (success) u.searchParams.set('success', success); else u.searchParams.delete('success');
    location.assign(u.toString());
  }

  /* ── Notices ────────────────────────────────────────────────────────── */
  function setupNotices() {
    const notices = $$('[data-notice]');
    notices.forEach((n) => on($('[data-dismiss]', n), 'click', () => n.remove()));
    if (!notices.length || !history.replaceState) return;
    const u = new URL(location.href);
    if (u.searchParams.has('success') || u.searchParams.has('error')) {
      u.searchParams.delete('success');
      u.searchParams.delete('error');
      history.replaceState(null, '', u.pathname + u.search + u.hash);
    }
  }

  /* ── Relative times ─────────────────────────────────────────────────── */
  const rtf = (typeof Intl !== 'undefined' && Intl.RelativeTimeFormat) ? new Intl.RelativeTimeFormat('en', { numeric: 'auto' }) : null;
  function relTime(date) {
    if (!rtf) return null;
    const diff = (date.getTime() - Date.now()) / 1000;
    const abs = Math.abs(diff);
    const units = [['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60]];
    for (const [unit, secs] of units) if (abs >= secs) return rtf.format(Math.round(diff / secs), unit);
    return abs < 30 ? 'just now' : rtf.format(Math.round(diff), 'second');
  }
  function setupTimes() {
    $$('time[data-rel]').forEach((t) => {
      const d = new Date(t.getAttribute('datetime'));
      if (isNaN(d.getTime())) return;
      const rel = relTime(d);
      if (!rel) return;
      t.title = d.toLocaleString();
      t.textContent = rel;
    });
  }

  /* ── Theme ──────────────────────────────────────────────────────────────
     The same contract as the rest of the family and as theme.js in <head>:
     localStorage.theme is "dark", "light" or "system", dark is the default,
     the resolved value is a class on <html> plus color-scheme. The header
     button flips between light and dark; the setting in Settings can also
     say "system". */
  const media = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)') : null;
  // A ?theme= in the address wins for this page load, as theme.js already
  // honoured before the first paint, until the person changes the theme.
  let forced = (() => {
    try { const v = new URLSearchParams(location.search).get('theme'); return v === 'light' || v === 'dark' ? v : null; }
    catch (e) { return null; }
  })();
  const storedTheme = () => {
    if (forced) return forced;
    try { const v = localStorage.getItem('theme'); return v === 'light' || v === 'dark' || v === 'system' ? v : 'dark'; }
    catch (e) { return 'dark'; }
  };
  const resolveTheme = (t) => (t === 'system' ? (media && media.matches ? 'dark' : 'light') : t);
  function applyTheme() {
    const resolved = resolveTheme(storedTheme());
    const root = document.documentElement;
    root.classList.remove('light', 'dark');
    root.classList.add(resolved);
    root.style.colorScheme = resolved;
    const btn = $('#theme-toggle');
    if (btn) {
      btn.setAttribute('aria-label', resolved === 'dark' ? 'Switch to light theme' : 'Switch to dark theme');
      btn.title = resolved === 'dark' ? 'Light theme' : 'Dark theme';
    }
  }
  function setTheme(t) {
    forced = null;
    try { localStorage.setItem('theme', t); } catch (e) { /* private mode: the choice lasts the page */ }
    applyTheme();
    const seg = $('#theme-seg');
    if (seg) $$('input', seg).forEach((r) => { r.checked = r.value === t; });
    if (seg) syncSegment(seg);
  }
  function setupTheme() {
    applyTheme();
    on($('#theme-toggle'), 'click', () => setTheme(resolveTheme(storedTheme()) === 'dark' ? 'light' : 'dark'));
    const seg = $('#theme-seg');
    if (seg) {
      const current = storedTheme();
      $$('input', seg).forEach((r) => { r.checked = r.value === current; });
      on(seg, 'change', (e) => { if (e.target && e.target.value) setTheme(e.target.value); });
    }
    if (media) on(media, 'change', () => { if (storedTheme() === 'system') applyTheme(); });
  }

  /* ── Segmented controls: a thumb that slides under the chosen option ── */
  function syncSegment(seg) {
    const opts = $$('.seg-opt', seg);
    const index = Math.max(0, opts.findIndex((o) => { const r = $('input', o); return r && r.checked; }));
    seg.style.setProperty('--n', String(opts.length));
    seg.style.setProperty('--i', String(index));
  }
  function setupSegments() {
    $$('.seg').forEach((seg) => {
      if (!$('.seg-thumb', seg)) {
        const thumb = el('span', 'seg-thumb');
        thumb.setAttribute('aria-hidden', 'true');
        seg.prepend(thumb);
      }
      seg.dataset.js = 'true';
      syncSegment(seg);
      on(seg, 'change', () => syncSegment(seg));
    });
  }

  /* ── Account menu ───────────────────────────────────────────────────── */
  function setupAccountMenu() {
    const d = $('#account-menu');
    if (!d) return;
    on(document, 'click', (e) => { if (d.open && !d.contains(e.target)) d.removeAttribute('open'); });
    on(document, 'keydown', (e) => {
      if (e.key === 'Escape' && d.open) { d.removeAttribute('open'); const s = $('summary', d); if (s) s.focus(); }
    });
  }

  /* ── Copy ───────────────────────────────────────────────────────────── */
  async function copyText(text) {
    try { await navigator.clipboard.writeText(text); return true; }
    catch (e) {
      const ta = el('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.className = 'visually-hidden';
      document.body.appendChild(ta);
      ta.select();
      let ok = false;
      try { ok = document.execCommand('copy'); } catch (e2) { ok = false; }
      ta.remove();
      return ok;
    }
  }
  function setupCopy() {
    on(document, 'click', async (e) => {
      const btn = e.target.closest('.copy-btn');
      if (!btn) return;
      const text = btn.getAttribute('data-copy');
      if (!text) return;
      const ok = await copyText(text);
      if (!ok) { toast('Could not copy', 'error', text); return; }
      toast('Copied', 'success');
      const use = btn.querySelector('use');
      if (use) {
        const was = use.getAttribute('href');
        use.setAttribute('href', '#i-check');
        btn.classList.add('is-done');
        setTimeout(() => { use.setAttribute('href', was); btn.classList.remove('is-done'); }, 1600);
      }
    });
  }

  /* ── The URL, shown as what stays and what goes ─────────────────────── */
  function parseURL(raw) {
    let s = raw.trim();
    if (!s) return null;
    if (!/^https?:\/\//i.test(s)) {
      if (/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) return null;
      s = 'https://' + s;
    }
    try { return new URL(s); } catch (e) { return null; }
  }
  function renderDiff(target, raw, stripped) {
    target.textContent = '';
    const u = parseURL(raw);
    if (!u) { target.textContent = raw; return; }
    const gone = new Set(stripped.map((s) => s.toLowerCase()));
    target.appendChild(el('span', 'q-base', u.origin + u.pathname));
    Array.from(u.searchParams.entries()).forEach(([k, v], i) => {
      target.appendChild(el('span', 'q-sep', i === 0 ? '?' : '&'));
      target.appendChild(el('span', 'q ' + (gone.has(k.toLowerCase()) ? 'q-strip' : 'q-keep'), v === '' ? k : `${k}=${v}`));
    });
    if (u.hash) target.appendChild(el('span', 'q-base', u.hash));
  }
  function renderBadges(target, stripped) {
    target.textContent = '';
    stripped.forEach((p) => target.appendChild(el('span', 'q q-strip', p)));
  }
  const countText = (n) => (n === 0
    ? 'Nothing to strip: stored exactly as pasted.'
    : `${n} tracking ${n === 1 ? 'parameter' : 'parameters'} cut`);

  /* ── The cleaner, mirrored for the front page ───────────────────────────
     The demo on the front page runs here so that nothing typed into it is
     sent anywhere. The rules are those of internal/services/cleaner.go; the
     dashboard asks the server, which is the authority. */
  const TRACKERS = new Set([
    'utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content', 'utm_id', 'utm_source_platform',
    'utm_creative_format', 'utm_marketing_tactic', 'utm_reader', 'utm_name', 'utm_cid', 'utm_viz_id',
    'utm_pubreferrer', 'utm_swu',
    'gclid', 'gclsrc', 'dclid', 'gad_source', 'gbraid', 'wbraid', '_ga', '_gl',
    'fbclid', 'fbadid', 'fb_action_ids', 'fb_action_types', 'fb_source', 'fb_ref', 'action_object_map', 'igshid',
    'msclkid', 'twclid', 'ttclid', 'li_fat_id', 'epik', 'yclid', 'ym_debug', '_openstat',
    'mc_cid', 'mc_eid', '_hsenc', '_hsmi', 'hsa_cam', 'hsa_grp', 'hsa_mt', 'hsa_src', 'hsa_ad', 'hsa_acc',
    'hsa_net', 'hsa_kw', 'hsa_tgt', 'hsa_ol', 'mkt_tok', 'vero_id', 'vero_conv', '_kx', 'wickedid', 'si', 'feature',
  ]);
  const TRACKER_PREFIXES = ['utm_', 'hsa_', 'ref_', 'pf_rd_', 'pd_rd_'];
  const isTracker = (k) => { const key = k.toLowerCase(); return TRACKERS.has(key) || TRACKER_PREFIXES.some((p) => key.startsWith(p)); };
  function cleanLocally(raw) {
    const u = parseURL(raw);
    if (!u) return null;
    const stripped = [];
    const kept = [];
    for (const [k, v] of u.searchParams.entries()) {
      if (isTracker(k)) { if (!stripped.includes(k)) stripped.push(k); } else kept.push([k, v]);
    }
    stripped.sort();
    kept.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
    const clean = new URL(u.toString());
    clean.search = new URLSearchParams(kept).toString();
    return { clean: clean.toString(), stripped };
  }

  function setupDemo() {
    const input = $('#demo-url');
    if (!input) return;
    const box = $('#demo-strip');
    const orig = $('#demo-original');
    const clean = $('#demo-clean');
    const count = $('#demo-count');
    const badges = $('#demo-badges');
    const render = () => {
      const raw = input.value.trim();
      const r = cleanLocally(raw);
      if (!raw || !r) {
        box.classList.add('is-empty');
        box.classList.remove('is-clean');
        orig.textContent = raw;
        clean.textContent = raw ? 'Only http and https addresses are accepted.' : '';
        count.textContent = '';
        badges.textContent = '';
        return;
      }
      box.classList.remove('is-empty');
      box.classList.toggle('is-clean', r.stripped.length === 0);
      renderDiff(orig, raw, r.stripped);
      clean.textContent = r.clean;
      count.textContent = countText(r.stripped.length);
      renderBadges(badges, r.stripped);
    };
    on(input, 'input', render);
    $$('[data-demo]').forEach((b) => on(b, 'click', () => { input.value = b.getAttribute('data-demo'); render(); input.focus(); }));
    render();
  }

  /* ── The composer ───────────────────────────────────────────────────── */
  function setupComposer() {
    const form = $('#create-link-form');
    if (!form) return;
    const input = $('#url', form);
    const box = $('#strip-preview');
    const orig = $('#strip-original');
    const clean = $('#strip-clean');
    const count = $('#strip-count');
    const badges = $('#strip-badges');
    const hint = $('#strip-hint');
    let timer = 0;
    let seq = 0;
    const hide = () => { box.hidden = true; hint.hidden = true; };

    on(input, 'input', () => {
      clearTimeout(timer);
      const raw = input.value.trim();
      if (raw.length < 4) { hide(); return; }
      timer = setTimeout(async () => {
        const mine = ++seq;
        try {
          const data = await api('/api/clean-preview', { method: 'POST', body: { url: raw } });
          if (mine !== seq) return;
          const stripped = data.stripped_params || [];
          renderDiff(orig, raw, stripped);
          clean.textContent = data.clean_url;
          count.textContent = countText(stripped.length);
          renderBadges(badges, stripped);
          box.classList.toggle('is-clean', stripped.length === 0);
          box.hidden = false;
          hint.hidden = true;
        } catch (err) {
          if (mine !== seq) return;
          box.hidden = true;
          hint.textContent = '';
          hint.appendChild(icon('i-alert'));
          hint.appendChild(document.createTextNode(err.message));
          hint.hidden = false;
        }
      }, 220);
    });

    // The short link, as it will be.
    const slug = $('#custom_slug', form);
    const domainSel = $('#domain', form);
    const preview = $('#slug-preview');
    const defaultDomain = form.getAttribute('data-default-domain') || location.host;
    const renderSlug = () => {
      if (!preview) return;
      const d = (domainSel && domainSel.value) || defaultDomain;
      const s = slug ? slug.value.trim() : '';
      preview.textContent = `${d}/${s || '…'}`;
    };
    on(slug, 'input', renderSlug);
    on(domainSel, 'change', renderSlug);
    renderSlug();

    // "Each link will …": the summary follows the choices.
    const summary = $('#opts-summary', form);
    const setBadge = (name, text) => {
      const b = summary && $(`[data-summary="${name}"]`, summary);
      if (!b) return;
      Array.from(b.childNodes).forEach((n) => { if (n.nodeType === Node.TEXT_NODE) n.remove(); });
      b.appendChild(document.createTextNode(text));
    };
    const hoursLabel = (h) => (h >= 720 ? `${Math.round(h / 720)} month${h >= 1440 ? 's' : ''}` : h >= 168 ? `${Math.round(h / 168)} week${h >= 336 ? 's' : ''}` : h >= 24 ? `${Math.round(h / 24)} day${h >= 48 ? 's' : ''}` : `${h} h`);
    const renderSummary = () => {
      const exp = form.querySelector('input[name="expires_in_hours"]:checked');
      const max = form.querySelector('input[name="max_clicks"]:checked');
      const pin = $('#pin', form);
      const hours = exp && exp.value ? parseInt(exp.value, 10) : 0;
      setBadge('expires', hours ? `expire in ${hoursLabel(hours)}` : 'never expire');
      const n = max && max.value ? parseInt(max.value, 10) : 0;
      setBadge('clicks', n ? `${n.toLocaleString()} click${n === 1 ? '' : 's'} at most` : 'unlimited clicks');
      setBadge('pin', pin && pin.value.trim() ? 'ask for a PIN' : 'open to anyone with the link');
    };
    on(form, 'change', renderSummary);
    on($('#pin', form), 'input', renderSummary);
    renderSummary();

    on(form, 'submit', () => { const b = $('#create-submit', form); if (b) b.classList.add('is-busy'); });
  }

  /* ── The list ───────────────────────────────────────────────────────── */
  function setupLinks() {
    const list = $('#links');
    const search = $('#link-search');
    const count = $('#link-count');
    const empty = $('#search-empty');

    if (search && list) {
      const rows = $$('.fc', list);
      const total = rows.length;
      const apply = () => {
        const q = search.value.trim().toLowerCase();
        let shown = 0;
        rows.forEach((r) => {
          const hit = !q || (r.getAttribute('data-search') || '').toLowerCase().includes(q);
          r.hidden = !hit;
          if (hit) shown++;
        });
        if (count) count.textContent = q ? `${shown} of ${total}` : String(total);
        if (empty) empty.hidden = shown !== 0;
        list.hidden = shown === 0;
      };
      on(search, 'input', apply);
      on(search, 'keydown', (e) => { if (e.key === 'Escape') { search.value = ''; apply(); search.blur(); } });
    }
    if (search) {
      on(document, 'keydown', (e) => {
        if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
        const t = e.target;
        if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return;
        e.preventDefault();
        search.focus();
        search.select();
      });
    }

    on(document, 'click', async (e) => {
      const btn = e.target.closest('.delete-link-btn');
      if (!btn) return;
      const id = btn.getAttribute('data-id');
      const slug = btn.getAttribute('data-slug');
      const yes = await confirmDialog({
        title: `Delete /${slug}?`,
        message: 'The short link stops answering immediately. Anyone holding it will find it gone.',
        confirmLabel: 'Delete',
      });
      if (!yes) return;
      try {
        await api(`/api/links/${encodeURIComponent(id)}`, { method: 'DELETE' });
        reloadWith(`Deleted /${slug}`);
      } catch (err) { toast('Could not delete it', 'error', err.message); }
    });
  }

  /* ── QR ─────────────────────────────────────────────────────────────── */
  function setupQR() {
    const d = $('#qr-dialog');
    if (!d) return;
    wireDialog(d);
    const img = $('#qr-image');
    const png = $('#qr-png');
    const forge = $('#qr-forge-direct');
    const text = $('#qr-link-text');
    const copy = $('#qr-copy');
    on(document, 'click', (e) => {
      const btn = e.target.closest('.qr-btn');
      if (!btn) return;
      const id = btn.getAttribute('data-id');
      const url = btn.getAttribute('data-url');
      const title = btn.getAttribute('data-title') || '';
      const base = btn.getAttribute('data-qrforge') || '';
      if (img) { img.src = `/api/links/${encodeURIComponent(id)}/qr.svg`; img.alt = `QR code for ${url}`; }
      if (png) png.href = `/api/links/${encodeURIComponent(id)}/qr.png`;
      // QR-Forge gets the intention: the short URL, the title and where it
      // came from, so its form arrives filled in.
      if (forge && base) forge.href = `${base}/new?url=${encodeURIComponent(url)}&title=${encodeURIComponent(title)}&from=linkup`;
      if (text) text.textContent = url;
      if (copy) copy.setAttribute('data-copy', url);
      openDialog(d);
    });
  }

  /* ── Edit ───────────────────────────────────────────────────────────── */
  function setupEdit() {
    const d = $('#edit-modal');
    const form = $('#edit-form');
    if (!d || !form) return;
    wireDialog(d);
    const f = (id) => document.getElementById(id);
    const errorBox = f('edit-error');
    const pinRemoveRow = f('edit-pin-remove-row');
    const toLocalInput = (unix) => {
      if (!unix) return '';
      const dt = new Date(unix * 1000);
      const p = (n) => String(n).padStart(2, '0');
      return `${dt.getFullYear()}-${p(dt.getMonth() + 1)}-${p(dt.getDate())}T${p(dt.getHours())}:${p(dt.getMinutes())}`;
    };

    on(document, 'click', async (e) => {
      const btn = e.target.closest('.edit-link-btn');
      if (!btn) return;
      const id = btn.getAttribute('data-id');
      try {
        const link = await api(`/api/links/${encodeURIComponent(id)}`);
        f('edit-id').value = link.id;
        f('edit-slug').textContent = (link.domain || '') + '/' + link.slug;
        f('edit-url').value = link.target_url || '';
        f('edit-title-input').value = link.title || '';
        f('edit-folder').value = link.folder_id || '';
        f('edit-tags').value = (link.tags || []).join(', ');
        f('edit-redirect').value = String(link.redirect_type || 302);
        f('edit-pin').value = '';
        f('edit-pin').placeholder = link.has_pin ? 'Unchanged. Type a new one to replace it' : 'No PIN';
        f('edit-pin-remove').checked = false;
        pinRemoveRow.hidden = !link.has_pin;
        f('edit-expires').value = toLocalInput(link.expires_at);
        f('edit-max-clicks').value = link.max_clicks || '';
        f('edit-active').checked = link.is_active !== false;
        f('edit-ios').value = link.ios_url || '';
        f('edit-android').value = link.android_url || '';
        errorBox.hidden = true;
        f('edit-save').disabled = false;
        openDialog(d);
        f('edit-url').focus();
      } catch (err) { toast('Could not load the link', 'error', err.message); }
    });

    on(f('edit-cancel'), 'click', () => closeDialog(d));

    on(form, 'submit', async (e) => {
      e.preventDefault();
      const id = f('edit-id').value;
      const body = {
        target_url: f('edit-url').value.trim(),
        title: f('edit-title-input').value.trim(),
        folder_id: f('edit-folder').value,
        tags: f('edit-tags').value.split(',').map((t) => t.trim()).filter(Boolean),
        redirect_type: parseInt(f('edit-redirect').value, 10),
        max_clicks: parseInt(f('edit-max-clicks').value, 10) || 0,
        is_active: f('edit-active').checked,
        ios_url: f('edit-ios').value.trim(),
        android_url: f('edit-android').value.trim(),
      };
      const expires = f('edit-expires').value;
      body.expires_at = expires ? Math.floor(new Date(expires).getTime() / 1000) : 0;
      if (f('edit-pin-remove').checked) body.pin = '';
      else if (f('edit-pin').value.trim()) body.pin = f('edit-pin').value.trim();

      const save = f('edit-save');
      save.disabled = true;
      try {
        await api(`/api/links/${encodeURIComponent(id)}`, { method: 'PATCH', body });
        reloadWith('Changes saved');
      } catch (err) {
        errorBox.textContent = err.message;
        errorBox.hidden = false;
        save.disabled = false;
      }
    });
  }

  /* ── Folders ────────────────────────────────────────────────────────── */
  function setupFolders() {
    const d = $('#folder-dialog');
    const form = $('#folder-form');
    const addBtns = $$('.add-folder-btn');
    const renameBtn = $('#rename-folder-btn');
    const deleteBtn = $('#delete-folder-btn');

    if (d && form) {
      wireDialog(d);
      const name = $('#folder-name', d);
      const title = $('#folder-title', d);
      const save = $('#folder-save', d);
      const err = $('#folder-error', d);
      let mode = 'create';
      let id = '';
      const open = (m, data = {}) => {
        mode = m;
        id = data.id || '';
        title.textContent = m === 'create' ? 'New folder' : 'Rename folder';
        save.textContent = m === 'create' ? 'Create folder' : 'Save';
        save.disabled = false;
        name.value = data.name || '';
        const c = (data.color || '').toLowerCase();
        const radios = $$('input[name="color"]', d);
        let matched = false;
        radios.forEach((r) => { const hit = r.value.toLowerCase() === c; r.checked = hit; matched = matched || hit; });
        if (!matched && radios[0]) radios[0].checked = true;
        err.hidden = true;
        openDialog(d);
        name.focus();
        name.select();
      };
      addBtns.forEach((b) => on(b, 'click', () => open('create')));
      on(renameBtn, 'click', () => open('rename', {
        id: renameBtn.getAttribute('data-id'),
        name: renameBtn.getAttribute('data-name'),
        color: renameBtn.getAttribute('data-color'),
      }));
      on(form, 'submit', async (e) => {
        e.preventDefault();
        const n = name.value.trim();
        if (!n) { err.textContent = 'Give the folder a name.'; err.hidden = false; name.focus(); return; }
        const checked = form.querySelector('input[name="color"]:checked');
        const color = checked ? checked.value : '';
        save.disabled = true;
        try {
          if (mode === 'create') {
            const created = await api('/api/folders', { method: 'POST', body: { name: n, color } });
            reloadWith(`Folder “${n}” created`, `/?folder=${encodeURIComponent(created.id)}`);
          } else {
            await api(`/api/folders/${encodeURIComponent(id)}`, { method: 'PATCH', body: { name: n, color } });
            reloadWith('Folder updated');
          }
        } catch (ex) {
          err.textContent = ex.message;
          err.hidden = false;
          save.disabled = false;
        }
      });
    }

    // Deleting a folder never deletes a link: they go back to All links, and
    // the question says so, because that is what everybody asks.
    on(deleteBtn, 'click', async () => {
      const id = deleteBtn.getAttribute('data-id');
      const n = deleteBtn.getAttribute('data-name') || 'this folder';
      const c = $$('#links .fc').length;
      const links = c === 1 ? '1 link' : `${c} links`;
      const yes = await confirmDialog({
        title: `Delete the folder “${n}”?`,
        message: `Its ${links} are kept and go back to All links. Only the folder goes.`,
        confirmLabel: 'Delete folder',
      });
      if (!yes) return;
      try {
        await api(`/api/folders/${encodeURIComponent(id)}`, { method: 'DELETE' });
        reloadWith(`Folder “${n}” deleted. Its links are back in All links.`, '/');
      } catch (ex) { toast('Could not delete the folder', 'error', ex.message); }
    });
  }

  /* ── Settings ───────────────────────────────────────────────────────── */
  function setupSettings() {
    const kinds = [
      { sel: '.delete-api-key-btn', path: '/api/keys/', title: (n) => `Revoke the key “${n}”?`, msg: 'Anything still using it stops working immediately.', label: 'Revoke', done: 'API key revoked' },
      { sel: '.delete-domain-btn', path: '/api/domains/', title: (n) => `Remove ${n}?`, msg: 'New links can no longer be created under this domain.', label: 'Remove', done: 'Domain removed' },
      { sel: '.delete-webhook-btn', path: '/api/webhooks/', title: () => 'Delete this webhook?', msg: 'No more deliveries go to this endpoint.', label: 'Delete', done: 'Webhook deleted' },
    ];
    on(document, 'click', async (e) => {
      for (const k of kinds) {
        const btn = e.target.closest(k.sel);
        if (!btn) continue;
        const id = btn.getAttribute('data-id');
        const n = btn.getAttribute('data-name') || '';
        const yes = await confirmDialog({ title: k.title(n), message: k.msg, confirmLabel: k.label });
        if (!yes) return;
        try {
          await api(k.path + encodeURIComponent(id), { method: 'DELETE' });
          reloadWith(k.done, '/settings');
        } catch (ex) { toast('That did not work', 'error', ex.message); }
        return;
      }
    });

    // CSV import: a file picked or dropped, then the count of what happened.
    const input = $('#bulk-csv-upload');
    const zone = $('#dropzone');
    const result = $('#csv-result');
    if (input && zone && result) {
      const send = async (file) => {
        if (!file) return;
        zone.classList.add('is-busy');
        result.hidden = true;
        result.textContent = '';
        result.dataset.tone = '';
        const fd = new FormData();
        fd.append('file', file);
        try {
          const data = await api('/api/links/bulk-import', { method: 'POST', form: fd });
          const title = el('p', 'note-title');
          title.appendChild(icon('i-circle-check'));
          title.appendChild(document.createTextNode(`${data.total_created} created, ${data.total_skipped} skipped, ${data.total_processed} rows read.`));
          result.appendChild(title);
          if (data.errors && data.errors.length) {
            const ul = el('ul');
            data.errors.slice(0, 50).forEach((m) => ul.appendChild(el('li', null, m)));
            result.appendChild(ul);
          }
          const a = el('a', null, 'See the links');
          a.href = '/';
          result.appendChild(a);
          result.dataset.tone = 'ok';
          result.hidden = false;
          toast(`Imported ${data.total_created} ${data.total_created === 1 ? 'link' : 'links'}`, 'success');
        } catch (ex) {
          result.textContent = ex.message;
          result.dataset.tone = 'danger';
          result.hidden = false;
        } finally {
          zone.classList.remove('is-busy');
          input.value = '';
        }
      };
      on(input, 'change', () => send(input.files && input.files[0]));
      ['dragenter', 'dragover'].forEach((t) => on(zone, t, (e) => { e.preventDefault(); zone.classList.add('is-over'); }));
      ['dragleave', 'drop'].forEach((t) => on(zone, t, (e) => { e.preventDefault(); zone.classList.remove('is-over'); }));
      on(zone, 'drop', (e) => send(e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0]));
    }
  }

  /* ── Go ─────────────────────────────────────────────────────────────── */
  function init() {
    setupSegments();
    setupTheme();
    setupAccountMenu();
    setupNotices();
    setupTimes();
    setupCopy();
    wireDialog($('#confirm-dialog'));
    setupDemo();
    setupComposer();
    setupLinks();
    setupQR();
    setupEdit();
    setupFolders();
    setupSettings();
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init); else init();
})();
