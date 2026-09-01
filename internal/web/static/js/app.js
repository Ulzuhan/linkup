// LinkUp Client Application

document.addEventListener('DOMContentLoaded', () => {
  setupLiveCleanerPreview();
  setupClipboardCopy();
  setupQRModal();
  setupDeleteHandlers();
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
        prompt('Copy this link:', textToCopy);
      }
    });
  });
}

// QR Code Forge Modal
function setupQRModal() {
  const modal = document.getElementById('qr-modal');
  const modalClose = document.getElementById('qr-modal-close');
  const qrFrame = document.getElementById('qr-forge-frame');
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

      // Simple SVG QR preview via api or image
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

// Delete link
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

function escapeHtml(str) {
  return str.replace(/[&<>'"]/g, 
    tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
  );
}
