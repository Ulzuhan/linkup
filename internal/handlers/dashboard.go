package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/kaicorplabs/linkup/internal/config"
	"github.com/kaicorplabs/linkup/internal/models"
	"github.com/kaicorplabs/linkup/internal/services"
	"github.com/kaicorplabs/linkup/internal/web"
)

type DashboardHandler struct {
	cfg         *config.Config
	linkService *services.LinkService
	authService *services.AuthService
	renderer    *web.Renderer
}

func NewDashboardHandler(cfg *config.Config, linkService *services.LinkService, authService *services.AuthService, renderer *web.Renderer) *DashboardHandler {
	return &DashboardHandler{
		cfg:         cfg,
		linkService: linkService,
		authService: authService,
		renderer:    renderer,
	}
}

// Dashboard renders the main UI
func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	session, _ := h.authService.GetSession(r)

	var links []models.Link
	var totalClicks int
	var totalLinks int

	flashSuccess := r.URL.Query().Get("success")
	flashError := r.URL.Query().Get("error")

	currentUser := models.UserSession{}
	if session != nil {
		currentUser = *session
		userLinks, err := h.linkService.ListByUser(session.Username, session.IsAdmin)
		if err == nil {
			links = userLinks
			totalLinks = len(links)
			for _, l := range links {
				totalClicks += l.ClickCount
			}
		}
	}

	data := models.DashboardData{
		User:          currentUser,
		Links:         links,
		TotalLinks:    totalLinks,
		TotalClicks:   totalClicks,
		PublicHost:    h.cfg.PublicHost,
		DefaultDomain: h.cfg.DefaultDomain,
		QRForgeURL:    h.cfg.QRForgeURL,
		AccountURL:    h.cfg.AccountURL,
		EnrollURL:     h.cfg.EnrollURL,
		IsAdmin:       currentUser.IsAdmin,
		FlashSuccess:  flashSuccess,
		FlashError:    flashError,
	}

	w.WriteHeader(http.StatusOK)
	_ = h.renderer.Render(w, "dashboard.html", map[string]interface{}{
		"Title":         "Dashboard",
		"User":          data.User,
		"Links":         data.Links,
		"TotalLinks":    data.TotalLinks,
		"TotalClicks":   data.TotalClicks,
		"PublicHost":    data.PublicHost,
		"DefaultDomain": data.DefaultDomain,
		"QRForgeURL":    data.QRForgeURL,
		"AccountURL":    data.AccountURL,
		"EnrollURL":     data.EnrollURL,
		"IsAdmin":       data.IsAdmin,
		"FlashSuccess":  data.FlashSuccess,
		"FlashError":    data.FlashError,
	})
}

// HandleCreateForm handles POST /links/create from standard HTML form
func (h *DashboardHandler) HandleCreateForm(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.GetSession(r)
	if err != nil || session == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()

	urlInput := strings.TrimSpace(r.FormValue("url"))
	customSlug := strings.TrimSpace(r.FormValue("custom_slug"))
	title := strings.TrimSpace(r.FormValue("title"))
	pin := strings.TrimSpace(r.FormValue("pin"))

	redirectType := 302
	if r.FormValue("redirect_type") == "301" {
		redirectType = 301
	}

	var maxClicks *int
	if maxClicksStr := r.FormValue("max_clicks"); maxClicksStr != "" {
		if val, err := strconv.Atoi(maxClicksStr); err == nil && val > 0 {
			maxClicks = &val
		}
	}

	var expiresInHours *int
	if hoursStr := r.FormValue("expires_in_hours"); hoursStr != "" {
		if val, err := strconv.Atoi(hoursStr); err == nil && val > 0 {
			expiresInHours = &val
		}
	}

	req := models.CreateLinkRequest{
		URL:            urlInput,
		CustomSlug:     customSlug,
		Title:          title,
		PIN:            pin,
		RedirectType:   redirectType,
		MaxClicks:      maxClicks,
		ExpiresInHours: expiresInHours,
	}

	link, stripped, err := h.linkService.Create(req, session.Username)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/?error=%s", err.Error()), http.StatusSeeOther)
		return
	}

	successMsg := fmt.Sprintf("Clean short link created: /%s", link.Slug)
	if len(stripped) > 0 {
		successMsg += fmt.Sprintf(" (stripped %d surveillance parameters)", len(stripped))
	}

	http.Redirect(w, r, fmt.Sprintf("/?success=%s", successMsg), http.StatusSeeOther)
}
