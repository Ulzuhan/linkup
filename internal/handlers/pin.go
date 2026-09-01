package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
	"github.com/go-chi/chi/v5"
)

type PinHandler struct {
	cfg         *config.Config
	linkService *services.LinkService
	renderer    *web.Renderer
	// guard budgets attempts PER LINK, not per visitor. Per visitor would need
	// an address, and this product does not look at addresses. The link is the
	// thing being attacked anyway, so it is the right thing to protect.
	guard *services.PINGuard
}

func NewPinHandler(cfg *config.Config, linkService *services.LinkService, renderer *web.Renderer) *PinHandler {
	return &PinHandler{
		cfg:         cfg,
		linkService: linkService,
		renderer:    renderer,
		guard:       services.NewPINGuard(5, 15*time.Minute),
	}
}

// ShowForm handles GET /pin/:slug
func (h *PinHandler) ShowForm(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	domain := extractDomain(r.Host, h.cfg.PublicHost, h.cfg.DefaultDomain)
	link, err := h.linkService.GetBySlugRaw(domain, slug)
	if err != nil || link == nil {
		http.NotFound(w, r)
		return
	}

	if link.IsExpired() {
		w.WriteHeader(http.StatusGone)
		_ = h.renderer.Render(w, "error.html", map[string]interface{}{
			"Title":      "Link Expired",
			"Heading":    "410 - Link Expired",
			"Message":    link.ExpiryReason(),
			"StatusCode": 410,
			"QRForgeURL": h.cfg.QRForgeURL,
		})
		return
	}

	_ = h.renderer.Render(w, "pin.html", map[string]interface{}{
		"Title":      "Enter PIN - " + slug,
		"Slug":       slug,
		"QRForgeURL": h.cfg.QRForgeURL,
	})
}

// Verify handles POST /pin/:slug
func (h *PinHandler) Verify(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	domain := extractDomain(r.Host, h.cfg.PublicHost, h.cfg.DefaultDomain)
	link, err := h.linkService.GetBySlugRaw(domain, slug)
	if err != nil || link == nil {
		http.NotFound(w, r)
		return
	}

	if link.IsExpired() {
		w.WriteHeader(http.StatusGone)
		_ = h.renderer.Render(w, "error.html", map[string]interface{}{
			"Title":      "Link Expired",
			"Heading":    "410 - Link Expired",
			"Message":    link.ExpiryReason(),
			"StatusCode": 410,
			"QRForgeURL": h.cfg.QRForgeURL,
		})
		return
	}

	// Checked before the PIN is even read: an exhausted budget must not leak
	// whether the attempt would have been right.
	if locked, remaining := h.guard.Locked(link.ID); locked {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = h.renderer.Render(w, "pin.html", map[string]interface{}{
			"Title":      "Enter PIN - " + slug,
			"Slug":       slug,
			"Error":      "Too many attempts on this link. Try again in " + humanWait(remaining) + ".",
			"QRForgeURL": h.cfg.QRForgeURL,
		})
		return
	}

	_ = r.ParseForm()
	pinInput := strings.TrimSpace(r.FormValue("pin"))

	// The comparison itself is already safe: VerifyPIN uses bcrypt, which is
	// salted and constant-time. What was missing was a budget.
	if !services.VerifyPIN(pinInput, link.PinHash) {
		h.guard.Failed(link.ID)
		w.WriteHeader(http.StatusUnauthorized)
		_ = h.renderer.Render(w, "pin.html", map[string]interface{}{
			"Title":      "Enter PIN - " + slug,
			"Slug":       slug,
			"Error":      "Incorrect PIN code. Please try again.",
			"QRForgeURL": h.cfg.QRForgeURL,
		})
		return
	}

	// Valid PIN! Record click and redirect
	h.guard.Succeeded(link.ID)
	h.linkService.RecordClick(link.ID, link.Domain, slug, "")

	redirectCode := link.RedirectType
	if redirectCode != 301 && redirectCode != 302 {
		redirectCode = 302
	}

	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, link.TargetURL, redirectCode)
}

func extractDomain(hostHeader, publicHost, defaultDomain string) string {
	host := strings.ToLower(hostHeader)
	host = strings.Split(host, ":")[0]
	if host != "localhost" && host != "127.0.0.1" && host != publicHost && host != defaultDomain {
		return host
	}
	return ""
}

// humanWait renders a lockout in the coarsest unit that is still honest.
func humanWait(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds())+1)
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes())+1)
}
