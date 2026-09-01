package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
)

type SettingsHandler struct {
	cfg            *config.Config
	authService    *services.AuthService
	apiKeyService  *services.APIKeyService
	domainService  *services.DomainService
	webhookService *services.WebhookService
	renderer       *web.Renderer
}

func NewSettingsHandler(
	cfg *config.Config,
	authService *services.AuthService,
	apiKeyService *services.APIKeyService,
	domainService *services.DomainService,
	webhookService *services.WebhookService,
	renderer *web.Renderer,
) *SettingsHandler {
	return &SettingsHandler{
		cfg:            cfg,
		authService:    authService,
		apiKeyService:  apiKeyService,
		domainService:  domainService,
		webhookService: webhookService,
		renderer:       renderer,
	}
}

// ShowSettings renders GET /settings
func (h *SettingsHandler) ShowSettings(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.GetSession(r)
	if err != nil || session == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	flashSuccess := r.URL.Query().Get("success")
	flashError := r.URL.Query().Get("error")
	newKeySecret := r.URL.Query().Get("new_key")

	apiKeys, _ := h.apiKeyService.List(session.Username, session.IsAdmin)
	domains, _ := h.domainService.List(session.Username, session.IsAdmin)
	webhooks, _ := h.webhookService.List(session.Username, session.IsAdmin)

	data := models.SettingsData{
		User:          *session,
		APIKeys:       apiKeys,
		CustomDomains: domains,
		Webhooks:      webhooks,
		PublicHost:    h.cfg.PublicHost,
		DefaultDomain: h.cfg.DefaultDomain,
		QRForgeURL:    h.cfg.QRForgeURL,
		AccountURL:    h.cfg.AccountURL,
		EnrollURL:     h.cfg.EnrollURL,
		IsAdmin:       session.IsAdmin,
		FlashSuccess:  flashSuccess,
		FlashError:    flashError,
		NewAPIKey:     newKeySecret,
	}

	w.WriteHeader(http.StatusOK)
	_ = h.renderer.Render(w, "settings.html", map[string]interface{}{
		"Title":         "Settings & Integrations",
		"User":          data.User,
		"APIKeys":       data.APIKeys,
		"CustomDomains": data.CustomDomains,
		"Webhooks":      data.Webhooks,
		"PublicHost":    data.PublicHost,
		"DefaultDomain": data.DefaultDomain,
		"QRForgeURL":    data.QRForgeURL,
		"AccountURL":    data.AccountURL,
		"EnrollURL":     data.EnrollURL,
		"IsAdmin":       data.IsAdmin,
		"FlashSuccess":  data.FlashSuccess,
		"FlashError":    data.FlashError,
		"NewAPIKey":     data.NewAPIKey,
	})
}

// CreateAPIKeyForm handles POST /settings/keys
func (h *SettingsHandler) CreateAPIKeyForm(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.GetSession(r)
	if err != nil || session == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))

	_, secret, err := h.apiKeyService.Create(name, session.Username)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/settings?error=%s", err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/settings?success=API+key+created+successfully&new_key=%s", secret), http.StatusSeeOther)
}

// CreateDomainForm handles POST /settings/domains
func (h *SettingsHandler) CreateDomainForm(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.GetSession(r)
	if err != nil || session == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	domain := strings.TrimSpace(r.FormValue("domain"))

	_, err = h.domainService.Create(domain, session.Username)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/settings?error=%s", err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings?success=Custom+domain+added+successfully", http.StatusSeeOther)
}

// CreateWebhookForm handles POST /settings/webhooks
func (h *SettingsHandler) CreateWebhookForm(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.GetSession(r)
	if err != nil || session == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	url := strings.TrimSpace(r.FormValue("url"))
	secret := strings.TrimSpace(r.FormValue("secret"))
	events := r.Form["events"]

	req := models.CreateWebhookRequest{
		URL:    url,
		Secret: secret,
		Events: events,
	}

	_, err = h.webhookService.Create(req, session.Username)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/settings?error=%s", err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings?success=Webhook+endpoint+registered+successfully", http.StatusSeeOther)
}
