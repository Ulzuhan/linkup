package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/handlers"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
)

func main() {
	log.Println("==========================================================")
	log.Println("⚡ LinkUp v2 - Sovereign Redirect Engine & Privacy Gateway")
	log.Println("   KaiCorp Labs • Smart Routing • Multi-Domain • Webhooks")
	log.Println("==========================================================")

	// 1. Load configuration
	cfg := config.Load()

	// Process-wide destination policy, before anything can create a link.
	services.SetAllowPrivateTargets(cfg.AllowPrivateTargets)

	// 2. Initialize database
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize database: %v", err)
	}
	defer db.Close()

	// 3. Initialize high-speed in-memory multi-domain LRU cache
	cache := services.NewLinkCache(10000, 15*time.Minute)

	// 4. Initialize services
	webhookService := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhookService, cfg.PublicHost)
	domainService := services.NewDomainService(db)
	folderService := services.NewFolderService(db)
	// An API key carries no groups, so with group-based administration this is
	// always false: automation does not administer. That is deliberate — a
	// leaked key should not be able to purge domains — and it is why the
	// username fallback is passed with no groups rather than not passed at all.
	apiKeyService := services.NewAPIKeyService(db, func(username string) bool {
		return cfg.IsAdmin(username, nil)
	})
	csvService := services.NewCSVService(linkService)
	routerEngine := services.NewRouterEngine()
	authService := services.NewAuthService(cfg)

	// 5. Initialize web renderer
	renderer, err := web.NewRenderer()
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize web templates: %v", err)
	}

	// Lo que toda plantilla recibe sin que cada handler tenga que acordarse.
	renderer.SetCommon(map[string]interface{}{
		"ProviderName":  cfg.OIDCProviderName,
		"FooterLinks":   cfg.FooterLinks,
		"EnrollURL":     cfg.EnrollURL,
		"AccountURL":    cfg.AccountURL,
		"QRForgeURL":    cfg.QRForgeURL,
		"DefaultDomain": cfg.DefaultDomain,
	})

	// 6. Build HTTP Router
	router := handlers.NewRouter(
		cfg,
		linkService,
		domainService,
		folderService,
		apiKeyService,
		webhookService,
		csvService,
		routerEngine,
		authService,
		renderer,
	)

	// 7. Configure HTTP Server
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 8. Start server in goroutine
	go func() {
		log.Printf("[HTTP] LinkUp server listening on http://%s (Public Host: %s)", addr, cfg.PublicHost)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()

	// 9. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("[SHUTDOWN] Shutting down LinkUp server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[SHUTDOWN] Server forced to shutdown: %v", err)
	}

	log.Println("[SHUTDOWN] LinkUp stopped cleanly. Goodbye!")
}
