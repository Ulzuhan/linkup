package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/kaicorplabs/linkup/internal/config"
	"github.com/kaicorplabs/linkup/internal/models"
	"github.com/kaicorplabs/linkup/internal/services"
	"github.com/kaicorplabs/linkup/internal/web"
)

type RedirectHandler struct {
	cfg          *config.Config
	linkService  *services.LinkService
	routerEngine *services.RouterEngine
	renderer     *web.Renderer
}

func NewRedirectHandler(
	cfg *config.Config,
	linkService *services.LinkService,
	routerEngine *services.RouterEngine,
	renderer *web.Renderer,
) *RedirectHandler {
	return &RedirectHandler{
		cfg:          cfg,
		linkService:  linkService,
		routerEngine: routerEngine,
		renderer:     renderer,
	}
}

// Redirect handles GET /:slug
func (h *RedirectHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	// Extract domain from Host header
	host := strings.ToLower(r.Host)
	host = strings.Split(host, ":")[0] // strip port
	domain := ""
	if host != "localhost" && host != "127.0.0.1" && host != h.cfg.PublicHost && host != h.cfg.DefaultDomain {
		domain = host
	}

	link, err := h.linkService.Resolve(domain, slug)
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

	// Evaluate Smart Conditional Routing (Device / Locale / A-B Testing)
	resolvedURL, variantName := h.routerEngine.ResolveDestination(r, link)

	// Record click asynchronously without visitor profiling
	h.linkService.RecordClick(link.ID, link.Domain, slug, variantName)

	redirectCode := link.RedirectType
	if redirectCode != 301 && redirectCode != 302 {
		redirectCode = 302
	}

	// Set privacy-protecting headers
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate")
	http.Redirect(w, r, resolvedURL, redirectCode)
}

// Preview handles GET /preview/:slug
func (h *RedirectHandler) Preview(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	host := strings.ToLower(r.Host)
	host = strings.Split(host, ":")[0]
	domain := ""
	if host != "localhost" && host != "127.0.0.1" && host != h.cfg.PublicHost && host != h.cfg.DefaultDomain {
		domain = host
	}

	link, err := h.linkService.GetBySlugRaw(domain, slug)
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
		"Title":          "Safe Preview - " + link.Slug,
		"Slug":           data.Slug,
		"CleanURL":       data.CleanURL,
		"OriginalURL":    data.OriginalURL,
		"StrippedParams": data.StrippedParams,
		"TitleText":      data.Title,
		"HasPIN":         data.HasPIN,
		"IsExpired":      data.IsExpired,
		"ExpiryReason":   data.ExpiryReason,
		"QRForgeLink":    data.QRForgeLink,
		"BaseDomain":     data.BaseDomain,
		"QRForgeURL":     h.cfg.QRForgeURL,
		"AccountURL":     h.cfg.AccountURL,
		"EnrollURL":      h.cfg.EnrollURL,
		"User":           models.UserSession{},
	})
}
