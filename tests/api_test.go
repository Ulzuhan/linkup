package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/handlers"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
)

func setupTestServer(t *testing.T) (http.Handler, *services.LinkService, *services.AuthService, func()) {
	tmpDir, err := os.MkdirTemp("", "linkup-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	cfg := &config.Config{
		Port:          3464,
		PublicHost:    "localhost:3464",
		DefaultDomain: "localhost:3464",
		SessionSecret: []byte("super-secret-key-32-bytes-long!"),
		QRForgeURL:    "https://qr.kaicorplabs.com",
		DevMode:       true,
		AdminUsers:    map[string]bool{"admin": true},
	}

	cache := services.NewLinkCache(1000, 5*time.Minute)
	webhooks := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhooks, cfg.PublicHost)
	domainService := services.NewDomainService(db)
	folderService := services.NewFolderService(db)
	apiKeyService := services.NewAPIKeyService(db, func(u string) bool { return cfg.IsAdmin(u, nil) })
	csvService := services.NewCSVService(linkService)
	routerEngine := services.NewRouterEngine()
	authService := services.NewAuthService(cfg)
	renderer, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("failed to init renderer: %v", err)
	}

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

	cleanup := func() {
		db.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return router, linkService, authService, cleanup
}

func TestHealthCheck(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), `"status":"healthy"`) {
		t.Errorf("expected healthy status, got %s", rr.Body.String())
	}
}

func TestAPICleanPreview(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]string{
		"url": "https://example.com/item?utm_source=email&fbclid=1234&item=42",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/clean-preview", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var res models.CleanPreviewResult
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if res.CleanURL != "https://example.com/item?item=42" {
		t.Errorf("expected clean URL with item=42, got %s", res.CleanURL)
	}

	if len(res.StrippedParams) != 2 {
		t.Errorf("expected 2 stripped params, got %d", len(res.StrippedParams))
	}
}

func TestHTTPRedirectFlow(t *testing.T) {
	router, linkService, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Create short link
	_, _, err := linkService.Create(models.CreateLinkRequest{
		URL:          "https://github.com/kaicorplabs?utm_source=twitter",
		CustomSlug:   "gh",
		RedirectType: 302,
	}, "test-user")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Make HTTP GET /gh
	req := httptest.NewRequest("GET", "/gh", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected 302 Found, got %d", rr.Code)
	}

	loc := rr.Header().Get("Location")
	if loc != "https://github.com/kaicorplabs" {
		t.Errorf("expected clean redirect Location header, got %s", loc)
	}

	// Verify privacy headers
	if rr.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("expected Referrer-Policy no-referrer")
	}
}

// A link that has ended is not a link that never existed: the visitor gets
// 410 and the reason, not 404. This used to answer 404 for every ended link
// because Resolve returns the link together with an error and the handler
// read the error first.
func TestHTTPEndedLinkAnswers410WithReason(t *testing.T) {
	router, linkService, _, cleanup := setupTestServer(t)
	defer cleanup()

	patch := func(t *testing.T, id, body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/api/links/"+id, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req) // dev mode: a session for free
		if rr.Code != http.StatusOK {
			t.Fatalf("PATCH %s: %d %s", body, rr.Code, rr.Body.String())
		}
	}
	past := time.Now().Add(-1 * time.Hour).Unix()
	two := 2

	for _, tc := range []struct {
		name      string
		slug      string
		maxClicks *int
		end       func(t *testing.T, link *models.Link)
		want      string
	}{
		{
			name: "expires_at in the past",
			slug: "ended-by-time",
			end:  func(t *testing.T, l *models.Link) { patch(t, l.ID, fmt.Sprintf(`{"expires_at":%d}`, past)) },
			want: "This link has expired based on its scheduled time limit.",
		},
		{
			name: "paused by its owner",
			slug: "ended-by-pause",
			end:  func(t *testing.T, l *models.Link) { patch(t, l.ID, `{"is_active":false}`) },
			want: "This link has been deactivated by its owner.",
		},
		{
			name:      "click budget spent",
			slug:      "ended-by-budget",
			maxClicks: &two,
			end: func(t *testing.T, l *models.Link) {
				// The second and last click of the budget. Clicks are
				// recorded asynchronously, so the 410 is awaited below.
				if rr := get(t, router, "/"+l.Slug); rr.Code != http.StatusFound {
					t.Fatalf("last click: %d, want 302", rr.Code)
				}
			},
			want: "This link has self-destructed after reaching its maximum click budget.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link, _, err := linkService.Create(models.CreateLinkRequest{
				URL:        "https://example.com/" + tc.slug,
				CustomSlug: tc.slug,
				MaxClicks:  tc.maxClicks,
			}, "dev-user-id")
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if rr := get(t, router, "/"+tc.slug); rr.Code != http.StatusFound {
				t.Fatalf("before ending: %d, want 302", rr.Code)
			}

			tc.end(t, link)

			// The click that ends the budget lands asynchronously, so the
			// first 410 is awaited. Then twice on purpose: an update refreshes
			// the link in the cache, so one request finds it there and Resolve
			// evicts it, and the next goes to the database. Both paths must
			// be 410.
			deadline := time.Now().Add(5 * time.Second)
			rr := get(t, router, "/"+tc.slug)
			for rr.Code != http.StatusGone && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
				rr = get(t, router, "/"+tc.slug)
			}
			for i, rr := range []*httptest.ResponseRecorder{rr, get(t, router, "/"+tc.slug)} {
				if rr.Code != http.StatusGone {
					t.Fatalf("request %d: %d, want 410", i+1, rr.Code)
				}
				body := rr.Body.String()
				if !strings.Contains(body, "410 - Link Expired") || !strings.Contains(body, tc.want) {
					t.Errorf("request %d: page lacks the reason %q", i+1, tc.want)
				}
				if strings.Contains(body, "does not exist") {
					t.Errorf("request %d: page reads as a missing link", i+1)
				}
			}
		})
	}

	// A slug nobody ever created is still a 404.
	rr := get(t, router, "/never-existed")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing link: %d, want 404", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "does not exist") {
		t.Errorf("missing link page lacks the 404 wording")
	}
}

func TestHTTPPINProtectionFlow(t *testing.T) {
	router, linkService, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Create protected link
	_, _, err := linkService.Create(models.CreateLinkRequest{
		URL:        "https://secret.com/docs",
		CustomSlug: "docs-pin",
		PIN:        "9988",
	}, "test-user")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// 1. Direct GET /docs-pin should redirect to /pin/docs-pin
	req1 := httptest.NewRequest("GET", "/docs-pin", nil)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusSeeOther {
		t.Errorf("expected 303 See Other redirecting to PIN, got %d", rr1.Code)
	}
	if rr1.Header().Get("Location") != "/pin/docs-pin" {
		t.Errorf("expected redirect to /pin/docs-pin, got %s", rr1.Header().Get("Location"))
	}

	// 2. Submit wrong PIN
	formWrong := url.Values{"pin": {"1111"}}
	req2 := httptest.NewRequest("POST", "/pin/docs-pin", strings.NewReader(formWrong.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized on bad PIN, got %d", rr2.Code)
	}

	// 3. Submit correct PIN
	formCorrect := url.Values{"pin": {"9988"}}
	req3 := httptest.NewRequest("POST", "/pin/docs-pin", strings.NewReader(formCorrect.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr3 := httptest.NewRecorder()
	router.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusFound {
		t.Errorf("expected 302 Found redirect on valid PIN, got %d", rr3.Code)
	}
	if rr3.Header().Get("Location") != "https://secret.com/docs" {
		t.Errorf("expected destination redirect, got %s", rr3.Header().Get("Location"))
	}
}
