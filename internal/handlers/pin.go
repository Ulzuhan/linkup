package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/kaicorplabs/linkup/internal/config"
	"github.com/kaicorplabs/linkup/internal/services"
	"github.com/kaicorplabs/linkup/internal/web"
)

type PinHandler struct {
	cfg         *config.Config
	linkService *services.LinkService
	renderer    *web.Renderer
}

func NewPinHandler(cfg *config.Config, linkService *services.LinkService, renderer *web.Renderer) *PinHandler {
	return &PinHandler{
		cfg:         cfg,
		linkService: linkService,
		renderer:    renderer,
	}
}

// ShowForm handles GET /pin/:slug
func (h *PinHandler) ShowForm(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	link, err := h.linkService.GetBySlugRaw(slug)
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

	link, err := h.linkService.GetBySlugRaw(slug)
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

	_ = r.ParseForm()
	pinInput := strings.TrimSpace(r.FormValue("pin"))

	if !services.VerifyPIN(pinInput, link.PinHash) {
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
	h.linkService.RecordClick(link.ID, slug)

	redirectCode := link.RedirectType
	if redirectCode != 301 && redirectCode != 302 {
		redirectCode = 302
	}

	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, link.TargetURL, redirectCode)
}
