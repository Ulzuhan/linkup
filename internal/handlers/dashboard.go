package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
)

type DashboardHandler struct {
	cfg           *config.Config
	linkService   *services.LinkService
	domainService *services.DomainService
	folderService *services.FolderService
	authService   *services.AuthService
	renderer      *web.Renderer
}

func NewDashboardHandler(
	cfg *config.Config,
	linkService *services.LinkService,
	domainService *services.DomainService,
	folderService *services.FolderService,
	authService *services.AuthService,
	renderer *web.Renderer,
) *DashboardHandler {
	return &DashboardHandler{
		cfg:           cfg,
		linkService:   linkService,
		domainService: domainService,
		folderService: folderService,
		authService:   authService,
		renderer:      renderer,
	}
}

// Dashboard renders the main UI
func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	session, _ := h.authService.GetSession(r)

	var links []models.Link
	var folders []models.Folder
	var domains []models.CustomDomain
	var totalClicks int
	var totalLinks int

	flashSuccess := r.URL.Query().Get("success")
	flashError := r.URL.Query().Get("error")
	folderFilter := r.URL.Query().Get("folder")
	tagFilter := r.URL.Query().Get("tag")

	currentUser := models.UserSession{}
	if session != nil {
		currentUser = *session
		userLinks, err := h.linkService.ListByUser(session.Username, session.IsAdmin)
		if err == nil {
			folders, _ = h.folderService.List(session.Username, session.IsAdmin)
			domains, _ = h.domainService.List(session.Username, session.IsAdmin)

			for _, l := range userLinks {
				totalClicks += l.ClickCount

				// Apply folder filter
				if folderFilter != "" && (l.FolderID == nil || *l.FolderID != folderFilter) {
					continue
				}

				// Apply tag filter
				if tagFilter != "" {
					hasTag := false
					for _, t := range l.Tags {
						if strings.EqualFold(t, tagFilter) {
							hasTag = true
							break
						}
					}
					if !hasTag {
						continue
					}
				}

				links = append(links, l)
			}
			totalLinks = len(userLinks)
		}
	}

	data := models.DashboardData{
		User:          currentUser,
		Links:         links,
		Folders:       folders,
		CustomDomains: domains,
		CurrentFolder: folderFilter,
		CurrentTag:    tagFilter,
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
		"Folders":       data.Folders,
		"CustomDomains": data.CustomDomains,
		"CurrentFolder": data.CurrentFolder,
		"CurrentTag":    data.CurrentTag,
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
	domain := strings.TrimSpace(r.FormValue("domain"))
	title := strings.TrimSpace(r.FormValue("title"))
	pin := strings.TrimSpace(r.FormValue("pin"))
	iosURL := strings.TrimSpace(r.FormValue("ios_url"))
	androidURL := strings.TrimSpace(r.FormValue("android_url"))
	tagsStr := strings.TrimSpace(r.FormValue("tags"))
	folderIDStr := strings.TrimSpace(r.FormValue("folder_id"))

	var folderID *string
	if folderIDStr != "" {
		folderID = &folderIDStr
	}

	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			cleaned := strings.TrimSpace(t)
			if cleaned != "" {
				tags = append(tags, cleaned)
			}
		}
	}

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

	// Parse A/B variants if present
	var abVariants []models.ABVariant
	abA := strings.TrimSpace(r.FormValue("ab_url_a"))
	abB := strings.TrimSpace(r.FormValue("ab_url_b"))
	if abA != "" && abB != "" {
		weightA, _ := strconv.Atoi(r.FormValue("ab_weight_a"))
		if weightA <= 0 {
			weightA = 50
		}
		weightB := 100 - weightA
		if weightB <= 0 {
			weightB = 50
		}
		abVariants = append(abVariants, models.ABVariant{Name: "Variant A", TargetURL: abA, Weight: weightA})
		abVariants = append(abVariants, models.ABVariant{Name: "Variant B", TargetURL: abB, Weight: weightB})
	}

	req := models.CreateLinkRequest{
		URL:            urlInput,
		CustomSlug:     customSlug,
		Domain:         domain,
		Title:          title,
		FolderID:       folderID,
		Tags:           tags,
		PIN:            pin,
		RedirectType:   redirectType,
		MaxClicks:      maxClicks,
		ExpiresInHours: expiresInHours,
		IOSURL:         iosURL,
		AndroidURL:     androidURL,
		ABVariants:     abVariants,
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
