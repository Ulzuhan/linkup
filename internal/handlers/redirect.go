package handlers

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kaicorplabs/linkup/internal/config"
	"github.com/kaicorplabs/linkup/internal/models"
	"github.com/kaicorplabs/linkup/internal/services"
	"github.com/kaicorplabs/linkup/internal/web"
)

type RedirectHandler struct {
	cfg         *config.Config
	linkService *services.LinkService
	renderer    *web.Renderer
}

func NewRedirectHandler(cfg *config.Config, linkService *services.LinkService, renderer *web.Renderer) *RedirectHandler {
	return &RedirectHandler{
		cfg:         cfg,
		linkService: linkService,
		renderer:    renderer,
	}
}

// Redirect handles GET /:slug
func (h *RedirectHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	link, err := h.linkService.Resolve(slug)
	if err != nil || link == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = h.renderer.Render(w, "error.html", map[string]interface{}{
			"Title":      "Link Not Found",
			"Heading":    "404 - Link Not Found",
			"Message":    "The shortened link you requested does not exist or has been removed.",
			"StatusCode": 404,
			"QRForgeURL": h.cfg.QRForgeURL,
		})
		return
	}

	// Check expiration
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

	// Check PIN protection
	if link.HasPIN {
		http.Redirect(w, r, "/pin/"+slug, http.StatusSeeOther)
		return
	}

	// Record click asynchronously without visitor profiling
	h.linkService.RecordClick(link.ID, slug)

	// Direct redirect
	redirectCode := link.RedirectType
	if redirectCode != 301 && redirectCode != 302 {
		redirectCode = 302
	}

	// Set privacy-protecting referrer policy on outgoing redirect
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate")
	http.Redirect(w, r, link.TargetURL, redirectCode)
}

// Preview handles GET /preview/:slug
func (h *RedirectHandler) Preview(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	link, err := h.linkService.GetBySlugRaw(slug)
	if err != nil || link == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = h.renderer.Render(w, "error.html", map[string]interface{}{
			"Title":      "Link Not Found",
			"Heading":    "404 - Link Not Found",
			"Message":    "The preview link you requested does not exist.",
			"StatusCode": 404,
			"QRForgeURL": h.cfg.QRForgeURL,
		})
		return
	}

	// Re-clean original URL to extract stripped parameters for display
	cleanURL, stripped, _ := services.CleanURL(link.OriginalURL, h.cfg.PublicHost)

	qrForgeLink := fmt.Sprintf("%s/?text=%s", h.cfg.QRForgeURL, link.TargetURL)

	data := models.PublicPreviewData{
		Slug:           link.Slug,
		CleanURL:       cleanURL,
		OriginalURL:    link.OriginalURL,
		StrippedParams: stripped,
		Title:          link.Title,
		HasPIN:         link.HasPIN,
		IsExpired:      link.IsExpired(),
		ExpiryReason:   link.ExpiryReason(),
		QRForgeLink:    qrForgeLink,
		BaseDomain:     h.cfg.DefaultDomain,
	}

	w.WriteHeader(http.StatusOK)
	_ = h.renderer.Render(w, "preview.html", map[string]interface{}{
		"Title":             "Safe Preview - " + link.Slug,
		"Slug":              data.Slug,
		"CleanURL":          data.CleanURL,
		"OriginalURL":       data.OriginalURL,
		"StrippedParams":    data.StrippedParams,
		"TitleText":         data.Title,
		"HasPIN":            data.HasPIN,
		"IsExpired":         data.IsExpired,
		"ExpiryReason":      data.ExpiryReason,
		"QRForgeLink":       data.QRForgeLink,
		"BaseDomain":        data.BaseDomain,
		"QRForgeURL":        h.cfg.QRForgeURL,
		"AccountURL":        h.cfg.AccountURL,
		"EnrollURL":         h.cfg.EnrollURL,
		"User":              models.UserSession{},
	})
}
