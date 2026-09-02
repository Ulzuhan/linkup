// LinkUp Client Application v2

document.addEventListener('DOMContentLoaded', () => {
  setupLiveCleanerPreview();
  setupClipboardCopy();
  setupQRModal();
  setupDeleteHandlers();
  setupFolderCreation();
  setupFolderManagement();
  setupEditModal();
  setupSettingsHandlers();
  setupBulkCSVUpload();
});

// Live tracker stripper inspection
function setupLiveCleanerPreview() {
  const urlInput = document.getElementById('target-url-input');
  const previewBox = document.getElementById('stripper-preview-box');
  const strippedTags = document.getElementById('stripped-tags');
  const cleanUrlDisplay = document.getElementById('clean-url-display');

  if (!urlInput || !previewBox) return;

  let debounceTimer;
  urlInput.addEventListener('input', (e) => {
    clearTimeout(debounceTimer);
    const val = e.target.value.trim();
    if (!val || val.length < 5) {
      previewBox.hidden = true;
      return;
    }

    debounceTimer = setTimeout(async () => {
      try {
        const res = await fetch('/api/clean-preview', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: val })
        });
        if (!res.ok) return;
        const data = await res.json();
        
        if (data.stripped_params && data.stripped_params.length > 0) {
          previewBox.hidden = false;
          strippedTags.innerHTML = data.stripped_params.map(p => 
            `<span class="tracker-badge">✂️ ${escapeHtml(p)}</span>`
          ).join('');
          cleanUrlDisplay.textContent = data.clean_url;
        } else {
          previewBox.hidden = true;
        }
      } catch (err) {
        console.error('Preview error:', err);
      }
    }, 250);
  });
}

// Copy to clipboard with toast
function setupClipboardCopy() {
  document.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const textToCopy = btn.getAttribute('data-copy');
      if (!textToCopy) return;

      try {
        await navigator.clipboard.writeText(textToCopy);
        const originalText = btn.innerHTML;
        btn.innerHTML = '✓ Copied!';
        btn.classList.add('btn-primary');
        setTimeout(() => {
          btn.innerHTML = originalText;
          btn.classList.remove('btn-primary');
        }, 2000);
      } catch (err) {
        prompt('Copy this text:', textToCopy);
      }
    });
  });
}

// QR Code Forge Modal
function setupQRModal() {
  const modal = document.getElementById('qr-modal');
  const modalClose = document.getElementById('qr-modal-close');
  const qrLinkDirect = document.getElementById('qr-forge-direct');
  const qrLinkText = document.getElementById('qr-link-text');
  const qrImage = document.getElementById('qr-image');
  const qrPng = document.getElementById('qr-png');

  if (!modal) return;

  document.querySelectorAll('.qr-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const linkUrl = btn.getAttribute('data-url');
      const linkId = btn.getAttribute('data-id');
      const linkTitle = btn.getAttribute('data-title') || '';
      const qrForgeBase = btn.getAttribute('data-qrforge') || '';

      // El QR lo dibuja este servidor, en /api/links/<id>/qr.svg. Antes se le
      // pedía a un tercero con la URL corta dentro de la query, que era la
      // excepción más ruidosa de un producto cuyo argumento es que no habla con
      // nadie.
      if (qrImage && linkId) {
        qrImage.src = `/api/links/${encodeURIComponent(linkId)}/qr.svg`;
        qrImage.alt = `QR code for ${linkUrl}`;
      }
      if (qrPng && linkId) {
        qrPng.href = `/api/links/${encodeURIComponent(linkId)}/qr.png`;
      }

      // A QR-Forge se va con la intención puesta: URL, título y origen. Así el
      // formulario llega relleno en vez de pedir que se copie y se pegue algo
      // que ya tenemos aquí.
      if (qrLinkDirect && qrForgeBase) {
        qrLinkDirect.href = `${qrForgeBase}/new?url=${encodeURIComponent(linkUrl)}`
          + `&title=${encodeURIComponent(linkTitle)}&from=linkup`;
      }
      if (qrLinkText) qrLinkText.textContent = linkUrl;

      modal.classList.add('active');
    });
  });

  if (modalClose) {
    modalClose.addEventListener('click', () => modal.classList.remove('active'));
  }

  modal.addEventListener('click', (e) => {
    if (e.target === modal) modal.classList.remove('active');
  });
}

// Delete handlers
function setupDeleteHandlers() {
  document.querySelectorAll('.delete-link-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.getAttribute('data-id');
      const slug = btn.getAttribute('data-slug');
      if (!confirm(`Are you sure you want to delete /${slug}? This action cannot be undone.`)) {
        return;
      }

      try {
        const res = await fetch(`/api/links/${id}`, { method: 'DELETE' });
        if (res.ok) {
          window.location.reload();
        } else {
          const data = await res.json();
          alert(data.error || 'Failed to delete link');
        }
      } catch (err) {
        alert('Network error while deleting link');
      }
    });
  });
}

// Folder creation
function setupFolderCreation() {
  const addFolderBtn = document.getElementById('add-folder-btn');
  if (!addFolderBtn) return;

  addFolderBtn.addEventListener('click', async () => {
    const name = prompt('Enter folder/project name:');
    if (!name || !name.trim()) return;

    try {
      const res = await fetch('/api/folders', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() })
      });
      if (res.ok) {
        window.location.reload();
      } else {
        const data = await res.json();
        alert(data.error || 'Failed to create folder');
      }
    } catch (err) {
      alert('Network error while creating folder');
    }
  });
}

// Settings Action Handlers
function setupSettingsHandlers() {
  // Delete API Key
  document.querySelectorAll('.delete-api-key-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.getAttribute('data-id');
      if (!confirm('Are you sure you want to revoke this API key?')) return;
      const res = await fetch(`/api/keys/${id}`, { method: 'DELETE' });
      if (res.ok) window.location.reload();
    });
  });

  // Delete Domain
  document.querySelectorAll('.delete-domain-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.getAttribute('data-id');
      if (!confirm('Are you sure you want to remove this custom domain?')) return;
      const res = await fetch(`/api/domains/${id}`, { method: 'DELETE' });
      if (res.ok) window.location.reload();
    });
  });

  // Delete Webhook
  document.querySelectorAll('.delete-webhook-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.getAttribute('data-id');
      if (!confirm('Are you sure you want to delete this webhook endpoint?')) return;
      const res = await fetch(`/api/webhooks/${id}`, { method: 'DELETE' });
      if (res.ok) window.location.reload();
    });
  });
}

// Bulk CSV Upload
function setupBulkCSVUpload() {
  const input = document.getElementById('bulk-csv-upload');
  const status = document.getElementById('bulk-csv-status');
  if (!input || !status) return;

  input.addEventListener('change', async (e) => {
    const file = e.target.files[0];
    if (!file) return;

    status.textContent = '⏳ Processing and cleaning URLs...';

    const formData = new FormData();
    formData.append('file', file);

    try {
      const res = await fetch('/api/links/bulk-import', {
        method: 'POST',
        body: formData
      });
      const data = await res.json();
      if (res.ok) {
        status.textContent = `✓ Created ${data.total_created} links (${data.total_skipped} skipped).`;
        setTimeout(() => window.location.href = '/', 1500);
      } else {
        status.textContent = `⚠ ${data.error || 'Upload failed'}`;
      }
    } catch (err) {
      status.textContent = '⚠ Network error during upload.';
    }
  });
}

function escapeHtml(str) {
  return str.replace(/[&<>'"]/g, 
    tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
  );
}

// The account menu is a <details>: it works with no script at all. This only
// closes it when you click somewhere else or press Escape, which a <details>
// does not do on its own.
document.addEventListener('click', (e) => {
  document.querySelectorAll('details.kc-account[open]').forEach((d) => {
    if (!d.contains(e.target)) d.removeAttribute('open');
  });
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') document.querySelectorAll('details.kc-account[open]').forEach((d) => d.removeAttribute('open'));
});

// Renaming and deleting the folder that is currently selected. Deleting a
// folder never deletes a link: the links go back to "All links", and the
// confirmation says so, because that is the question everybody asks.
function setupFolderManagement() {
  const renameBtn = document.getElementById('rename-folder-btn');
  const deleteBtn = document.getElementById('delete-folder-btn');

  if (renameBtn) {
    renameBtn.addEventListener('click', async () => {
      const id = renameBtn.getAttribute('data-id');
      const current = renameBtn.getAttribute('data-name') || '';
      const name = prompt('New name for this folder:', current);
      if (!name || !name.trim() || name.trim() === current) return;
      try {
        const res = await fetch(`/api/folders/${id}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: name.trim() })
        });
        if (res.ok) window.location.reload();
        else alert((await res.json()).error || 'Could not rename the folder');
      } catch (err) {
        alert('Network error while renaming the folder');
      }
    });
  }

  if (deleteBtn) {
    deleteBtn.addEventListener('click', async () => {
      const id = deleteBtn.getAttribute('data-id');
      const name = deleteBtn.getAttribute('data-name') || 'this folder';
      const count = document.querySelectorAll('.delete-link-btn').length;
      const links = count === 1 ? '1 link' : `${count} links`;
      if (!confirm(`Delete the folder "${name}"?\n\nIts ${links} are kept and go back to All links. Only the folder goes.`)) return;
      try {
        const res = await fetch(`/api/folders/${id}`, { method: 'DELETE' });
        if (res.ok) window.location.href = '/';
        else alert((await res.json()).error || 'Could not delete the folder');
      } catch (err) {
        alert('Network error while deleting the folder');
      }
    });
  }
}

// The edit dialog: everything about a link except its address. Moving a link
// between folders is just another field here.
function setupEditModal() {
  const modal = document.getElementById('edit-modal');
  const form = document.getElementById('edit-form');
  if (!modal || !form) return;

  const field = (id) => document.getElementById(id);
  const errorBox = field('edit-error');
  const pinRemoveRow = field('edit-pin-remove-row');
  const close = () => { modal.classList.remove('active'); errorBox.hidden = true; };

  const toLocalInput = (unix) => {
    if (!unix) return '';
    const d = new Date(unix * 1000);
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  };

  document.querySelectorAll('.edit-link-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.getAttribute('data-id');
      try {
        const res = await fetch(`/api/links/${id}`);
        if (!res.ok) { alert('Could not load this link'); return; }
        const link = await res.json();
        field('edit-id').value = link.id;
        field('edit-slug').textContent = (link.domain ? link.domain : '') + '/' + link.slug;
        field('edit-url').value = link.target_url || '';
        field('edit-title').value = link.title || '';
        field('edit-folder').value = link.folder_id || '';
        field('edit-tags').value = (link.tags || []).join(', ');
        field('edit-redirect').value = String(link.redirect_type || 302);
        field('edit-pin').value = '';
        field('edit-pin').placeholder = link.has_pin ? 'Unchanged — type a new one to replace it' : 'No PIN';
        field('edit-pin-remove').checked = false;
        pinRemoveRow.hidden = !link.has_pin;
        field('edit-expires').value = toLocalInput(link.expires_at);
        field('edit-max-clicks').value = link.max_clicks || '';
        field('edit-active').checked = link.is_active !== false;
        field('edit-ios').value = link.ios_url || '';
        field('edit-android').value = link.android_url || '';
        errorBox.hidden = true;
        modal.classList.add('active');
        field('edit-url').focus();
      } catch (err) {
        alert('Network error while loading the link');
      }
    });
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const id = field('edit-id').value;
    const body = {
      target_url: field('edit-url').value.trim(),
      title: field('edit-title').value.trim(),
      folder_id: field('edit-folder').value,
      tags: field('edit-tags').value.split(',').map(t => t.trim()).filter(Boolean),
      redirect_type: parseInt(field('edit-redirect').value, 10),
      max_clicks: parseInt(field('edit-max-clicks').value, 10) || 0,
      is_active: field('edit-active').checked,
      ios_url: field('edit-ios').value.trim(),
      android_url: field('edit-android').value.trim(),
    };
    const expires = field('edit-expires').value;
    body.expires_at = expires ? Math.floor(new Date(expires).getTime() / 1000) : 0;
    if (field('edit-pin-remove').checked) body.pin = '';
    else if (field('edit-pin').value.trim()) body.pin = field('edit-pin').value.trim();

    const save = field('edit-save');
    save.disabled = true;
    try {
      const res = await fetch(`/api/links/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      if (res.ok) { window.location.reload(); return; }
      const data = await res.json().catch(() => ({}));
      errorBox.textContent = data.error || 'Could not save the changes';
      errorBox.hidden = false;
    } catch (err) {
      errorBox.textContent = 'Network error while saving';
      errorBox.hidden = false;
    } finally {
      save.disabled = false;
    }
  });

  field('edit-cancel').addEventListener('click', close);
  modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && modal.classList.contains('active')) close(); });
}

