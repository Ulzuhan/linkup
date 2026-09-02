package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/handlers"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
)

// setupPublicServer is the server as an anonymous visitor meets it: no dev
// mode, so no session is handed out for free, and with the enrollment link
// and the footer links an operator would set.
func setupPublicServer(t *testing.T) (http.Handler, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "linkup-web-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	db, err := database.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cfg := &config.Config{
		Port:          3464,
		PublicHost:    "localhost:3464",
		DefaultDomain: "localhost:3464",
		SessionSecret: []byte("super-secret-key-32-bytes-long!"),
		QRForgeURL:    "https://qr.example.test",
		EnrollURL:     "https://idp.example.test/enroll/",
		FooterLinks:   true,
		DevMode:       false,
		AdminUsers:    map[string]bool{},
	}
	cache := services.NewLinkCache(100, time.Minute)
	webhooks := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhooks, cfg.PublicHost)
	renderer, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("renderer: %v", err)
	}
	renderer.SetCommon(map[string]interface{}{
		"ProviderName": "your provider",
		"FooterLinks":  cfg.FooterLinks,
		"EnrollURL":    cfg.EnrollURL,
	})
	router := handlers.NewRouter(
		cfg, linkService, services.NewDomainService(db), services.NewFolderService(db),
		services.NewAPIKeyService(db, func(string) bool { return false }),
		webhooks, services.NewCSVService(linkService), services.NewRouterEngine(),
		services.NewAuthService(cfg), renderer,
	)
	return router, func() { db.Close(); os.RemoveAll(tmpDir) }
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

// The stylesheet used to answer 404 in production while every test stayed
// green: the file server was mounted without stripping its prefix. This is
// the test that would have caught it.
func TestThemeAssetsAreServed(t *testing.T) {
	h, done := setupPublicServer(t)
	defer done()
	for path, wantType := range map[string]string{
		"/static/css/app.css":            "text/css",
		"/static/css/kaicorp.css":        "text/css",
		"/static/css/landing-polish.css": "text/css",
		"/static/js/app.js":              "javascript",
		"/static/kaicorp-mark.png":       "image/png",
	} {
		rr := get(t, h, path)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", path, rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, wantType) {
			t.Errorf("%s: content-type %q, want it to contain %q", path, ct, wantType)
		}
	}
}

// An anonymous visitor gets a front page, not an empty dashboard.
func TestAnonymousHomeIsAFrontPage(t *testing.T) {
	h, done := setupPublicServer(t)
	defer done()
	rr := get(t, h, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`class="kc-product-landing`,
		`href="/auth/login"`,
		`https://idp.example.test/enroll/`,
		`kc-card-grid`,
		`Built by <strong>KaiCorp Labs</strong>`,
		`aria-current="page">LinkUp`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("front page lacks %q", want)
		}
	}
	for _, unwanted := range []string{`id="create-link-form"`, `kc-account`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("front page shows %q, which belongs to a signed-in session", unwanted)
		}
	}
	if n := strings.Count(body, "<header"); n != 1 {
		t.Errorf("%d headers, want exactly one", n)
	}
	if n := strings.Count(body, "<footer"); n != 1 {
		t.Errorf("%d footers, want exactly one", n)
	}
}

// With a session the same address is the workspace, with the account menu in
// the shared header.
func TestSignedInHomeIsTheDashboard(t *testing.T) {
	h, _, _, done := setupTestServer(t) // dev mode: a session for free on loopback
	defer done()
	rr := get(t, h, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{`id="create-link-form"`, `class="kc-account"`, `href="/settings"`, `kc-workspace`} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard lacks %q", want)
		}
	}
	if strings.Contains(body, `class="kc-product-landing`) {
		t.Errorf("dashboard shows the front page")
	}
}
