package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

	// Static assets.
	//
	// StripPrefix is not optional: StaticFS hangs off a sub-FS already rooted at
	// "static", so without it the FileServer looks for static/css/app.css INSIDE
	// static/ and answers 404 to every stylesheet and script. The page still
	// renders — unstyled — so nothing fails loudly and no test that exercises
	// handlers notices. It shipped that way.
	r.Handle("/static/*", http.StripPrefix("/static/", web.StaticFS()))

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

	// One bucket for every write path, form posts included: 60 writes a minute
	// per identity is far above any human and far below a runaway script.
	writeLimit := WriteRateLimit(services.NewTokenBucket(60, time.Minute), authService, apiKeyService)

	// Auth routes
	r.Route("/auth", func(r chi.Router) {
		r.Get("/login", authHandler.Login)
		r.Get("/callback", authHandler.Callback)
		r.Get("/logout", authHandler.Logout)
		r.Get("/me", authHandler.Me)
	})

	// Web Dashboard
	r.Get("/", dashboardHandler.Dashboard)
	r.Get("/settings", settingsHandler.ShowSettings)

	// The form posts share the limiter with the API: same writes, same budget.
	r.Group(func(r chi.Router) {
		r.Use(writeLimit)
		r.Post("/links/create", dashboardHandler.HandleCreateForm)
		r.Post("/settings/keys", settingsHandler.CreateAPIKeyForm)
		r.Post("/settings/domains", settingsHandler.CreateDomainForm)
		r.Post("/settings/webhooks", settingsHandler.CreateWebhookForm)
	})

	// REST API routes
	r.Route("/api", func(r chi.Router) {
		r.Use(writeLimit)
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
// The security headers travel with the application, not with whatever proxy
// happens to be in front of it.
//
// This is a self-hostable product: someone running it behind their own Caddy
// gets the same protection as the instance we run, without having to know these
// exist. Two intersecting policies from two places would also be worse than one
// from here — a header set at the edge and another set here narrow each other in
// ways nobody can read off a single file.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	// 'unsafe-inline' is here for the 163 style= attributes spread across the
	// templates, not for inline <script>: there are none, and script-src stays
	// strict because of it. Moving those attributes into the stylesheet is
	// tidying for later, and it is what removes this.
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"form-action 'self'; " +
	"base-uri 'none'; " +
	"frame-ancestors 'none'"

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// DENY rather than SAMEORIGIN: nothing here is meant to be framed, and
		// frame-ancestors 'none' above says the same to browsers that read CSP.
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		// X-XSS-Protection is deliberately absent. It is obsolete, browsers
		// ignore it, and in the versions that did not it introduced its own
		// vulnerabilities. CSP is what does this job now.
		next.ServeHTTP(w, r)
	})
}
