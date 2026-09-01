package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/handlers"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
)

func TestAPIKeyAuthentication(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "linkup-apikey-test-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "keys.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		Port:          3464,
		PublicHost:    "localhost:3464",
		DefaultDomain: "localhost:3464",
		SessionSecret: []byte("super-secret-key-32-bytes-long!"),
		QRForgeURL:    "https://qr.kaicorplabs.com",
		DevMode:       false, // strict mode
	}

	cache := services.NewLinkCache(1000, 5*time.Minute)
	webhooks := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhooks, cfg.PublicHost)
	domainService := services.NewDomainService(db)
	folderService := services.NewFolderService(db)
	apiKeyService := services.NewAPIKeyService(db, cfg.IsAdmin)
	csvService := services.NewCSVService(linkService)
	routerEngine := services.NewRouterEngine()
	authService := services.NewAuthService(cfg)
	renderer, _ := web.NewRenderer()

	router := handlers.NewRouter(
		cfg,
		linkService,
		domainService,
		folderService,
		apiKeyService,
		webhooks,
		csvService,
		routerEngine,
		authService,
		renderer,
	)

	// 1. Create API key for user 'bot-user'
	apiKey, rawSecret, err := apiKeyService.Create("Slack Bot Key", "bot-user")
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	if len(rawSecret) < 20 {
		t.Errorf("expected long raw secret, got %s", rawSecret)
	}

	// 2. Request without auth -> 401 Unauthorized
	reqUnauth := httptest.NewRequest("GET", "/api/links", nil)
	rrUnauth := httptest.NewRecorder()
	router.ServeHTTP(rrUnauth, reqUnauth)
	if rrUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without auth, got %d", rrUnauth.Code)
	}

	// 3. Request with valid Bearer Token -> 200 OK
	reqAuth := httptest.NewRequest("GET", "/api/links", nil)
	reqAuth.Header.Set("Authorization", "Bearer "+rawSecret)
	rrAuth := httptest.NewRecorder()
	router.ServeHTTP(rrAuth, reqAuth)
	if rrAuth.Code != http.StatusOK {
		t.Errorf("expected 200 OK with Bearer token, got %d: %s", rrAuth.Code, rrAuth.Body.String())
	}

	// 4. Create link using Bearer Token
	createPayload := models.CreateLinkRequest{
		URL:        "https://example.com/bot-created-link",
		CustomSlug: "bot-link",
	}
	body, _ := json.Marshal(createPayload)
	reqCreate := httptest.NewRequest("POST", "/api/links", bytes.NewBuffer(body))
	reqCreate.Header.Set("Authorization", "Bearer "+rawSecret)
	reqCreate.Header.Set("Content-Type", "application/json")
	rrCreate := httptest.NewRecorder()
	router.ServeHTTP(rrCreate, reqCreate)

	if rrCreate.Code != http.StatusCreated {
		t.Errorf("expected 201 Created via API key, got %d: %s", rrCreate.Code, rrCreate.Body.String())
	}

	// 5. Revoke API key
	if err := apiKeyService.Delete(apiKey.ID, "bot-user", false); err != nil {
		t.Fatalf("failed to delete API key: %v", err)
	}

	// 6. Request with revoked key -> 401 Unauthorized
	reqRevoked := httptest.NewRequest("GET", "/api/links", nil)
	reqRevoked.Header.Set("Authorization", "Bearer "+rawSecret)
	rrRevoked := httptest.NewRecorder()
	router.ServeHTTP(rrRevoked, reqRevoked)
	if rrRevoked.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized after key revocation, got %d", rrRevoked.Code)
	}
}
