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
	dashboardHandler := NewDashboardHandler(cfg, linkService, authService, renderer)
	redirectHandler := NewRedirectHandler(cfg, linkService, renderer)
	pinHandler := NewPinHandler(cfg, linkService, renderer)
	apiHandler := NewAPIHandler(cfg, linkService, authService)

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

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Post("/clean-preview", apiHandler.CleanPreview)
		r.Get("/links", apiHandler.ListLinks)
		r.Post("/links", apiHandler.CreateLink)
		r.Get("/links/{id}", apiHandler.GetLink)
		r.Patch("/links/{id}", apiHandler.UpdateLink)
		r.Delete("/links/{id}", apiHandler.DeleteLink)
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

		// Omit static assets from noisy logs
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
