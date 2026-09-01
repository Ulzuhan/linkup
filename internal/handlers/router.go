package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kaicorplabs/linkup/internal/config"
	"github.com/kaicorplabs/linkup/internal/services"
	"github.com/kaicorplabs/linkup/internal/web"
)

func NewRouter(
	cfg *config.Config,
	linkService *services.LinkService,
	domainService *services.DomainService,
	folderService *services.FolderService,
	apiKeyService *services.APIKeyService,
	webhookService *services.WebhookService,
	csvService *services.CSVService,
	routerEngine *services.RouterEngine,
	authService *services.AuthService,
	renderer *web.Renderer,
) *chi.Mux {
	r := chi.NewRouter()

	// Base middlewares
	r.Use(middleware.Recoverer)
	r.Use(PrivacyRespectingLogger)
	r.Use(SecurityHeaders)

	// Static Assets (/static/*)
	r.Handle("/static/*", web.StaticFS())

	// Health Check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"linkup"}`))
	})
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"linkup"}`))
	})

	// Handlers
	authHandler := NewAuthHandler(cfg, authService)
	dashboardHandler := NewDashboardHandler(cfg, linkService, domainService, folderService, authService, renderer)
	settingsHandler := NewSettingsHandler(cfg, authService, apiKeyService, domainService, webhookService, renderer)
	redirectHandler := NewRedirectHandler(cfg, linkService, routerEngine, renderer)
	pinHandler := NewPinHandler(cfg, linkService, renderer)
	apiHandler := NewAPIHandler(cfg, linkService, authService, apiKeyService)
	domainHandler := NewDomainHandler(domainService, authService, apiKeyService)
	folderHandler := NewFolderHandler(folderService, authService, apiKeyService)
	apiKeyHandler := NewAPIKeyHandler(apiKeyService, authService)
	webhookHandler := NewWebhookHandler(webhookService, authService, apiKeyService)
	bulkHandler := NewBulkHandler(csvService, authService, apiKeyService)

	// Auth routes
	r.Route("/auth", func(r chi.Router) {
		r.Get("/login", authHandler.Login)
		r.Get("/callback", authHandler.Callback)
		r.Get("/logout", authHandler.Logout)
		r.Get("/me", authHandler.Me)
	})

	// Web Dashboard
	r.Get("/", dashboardHandler.Dashboard)
	r.Post("/links/create", dashboardHandler.HandleCreateForm)

	// Settings & Integrations
	r.Get("/settings", settingsHandler.ShowSettings)
	r.Post("/settings/keys", settingsHandler.CreateAPIKeyForm)
	r.Post("/settings/domains", settingsHandler.CreateDomainForm)
	r.Post("/settings/webhooks", settingsHandler.CreateWebhookForm)

	// REST API routes
	r.Route("/api", func(r chi.Router) {
		r.Post("/clean-preview", apiHandler.CleanPreview)

		// Links
		r.Get("/links", apiHandler.ListLinks)
		r.Post("/links", apiHandler.CreateLink)
		r.Get("/links/{id}", apiHandler.GetLink)
		r.Patch("/links/{id}", apiHandler.UpdateLink)
		r.Delete("/links/{id}", apiHandler.DeleteLink)

		// Bulk CSV
		r.Post("/links/bulk-import", bulkHandler.BulkImport)
		r.Get("/links/export", bulkHandler.ExportCSV)

		// Domains
		r.Get("/domains", domainHandler.List)
		r.Post("/domains", domainHandler.Create)
		r.Delete("/domains/{id}", domainHandler.Delete)

		// Folders
		r.Get("/folders", folderHandler.List)
		r.Post("/folders", folderHandler.Create)
		r.Delete("/folders/{id}", folderHandler.Delete)

		// API Keys
		r.Get("/keys", apiKeyHandler.List)
		r.Post("/keys", apiKeyHandler.Create)
		r.Delete("/keys/{id}", apiKeyHandler.Delete)

		// Webhooks
		r.Get("/webhooks", webhookHandler.List)
		r.Post("/webhooks", webhookHandler.Create)
		r.Delete("/webhooks/{id}", webhookHandler.Delete)
	})

	// PIN verification
	r.Get("/pin/{slug}", pinHandler.ShowForm)
	r.Post("/pin/{slug}", pinHandler.Verify)

	// Clean destination preview
	r.Get("/preview/{slug}", redirectHandler.Preview)

	// Fast Redirection Route (Catch-all slug)
	r.Get("/{slug}", redirectHandler.Redirect)

	return r
}

// PrivacyRespectingLogger logs request method, path, status and duration without recording visitor IP or Referrers
func PrivacyRespectingLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		if r.URL.Path == "/healthz" || r.URL.Path == "/health" {
			return
		}

		duration := time.Since(start)
		log.Printf("[HTTP] %s %s -> %d (%s)", r.Method, r.URL.Path, ww.Status(), duration)
	})
}

// SecurityHeaders applies essential security headers
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}
