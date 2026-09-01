// LinkUp Client Application v2

document.addEventListener('DOMContentLoaded', () => {
  setupLiveCleanerPreview();
  setupClipboardCopy();
  setupQRModal();
  setupDeleteHandlers();
  setupFolderCreation();
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
      previewBox.style.display = 'none';
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
          previewBox.style.display = 'block';
          strippedTags.innerHTML = data.stripped_params.map(p => 
            `<span class="tracker-badge">✂️ ${escapeHtml(p)}</span>`
          ).join('');
          cleanUrlDisplay.textContent = data.clean_url;
        } else {
          previewBox.style.display = 'none';
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

  if (!modal) return;

  document.querySelectorAll('.qr-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const linkUrl = btn.getAttribute('data-url');
      const qrForgeBase = btn.getAttribute('data-qrforge') || 'https://qr.kaicorplabs.com';
      
      const forgeUrl = `${qrForgeBase}/?text=${encodeURIComponent(linkUrl)}`;
      if (qrLinkDirect) qrLinkDirect.href = forgeUrl;
      if (qrLinkText) qrLinkText.textContent = linkUrl;

      const qrImg = document.getElementById('qr-image-preview');
      if (qrImg) {
        qrImg.src = `https://api.qrserver.com/v1/create-qr-code/?size=240x240&data=${encodeURIComponent(linkUrl)}&color=06b6d4&bgcolor=141c2e`;
      }

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
