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
		"AssetVersion": web.AssetVersion(),
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

// A new build is a new address for the stylesheet, and the cache headers
// match: versioned is immutable, anything else is short-lived.
func TestAssetsAreVersionedAndCacheable(t *testing.T) {
	h, done := setupPublicServer(t)
	defer done()
	v := web.AssetVersion()
	if len(v) != 12 {
		t.Fatalf("asset version %q, want 12 hex characters", v)
	}
	body := get(t, h, "/").Body.String()
	for _, want := range []string{"/static/css/app.css?v=" + v, "/static/js/app.js?v=" + v} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not reference %q", want)
		}
	}
	if cc := get(t, h, "/static/css/app.css?v="+v).Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("versioned asset Cache-Control %q, want immutable", cc)
	}
	if cc := get(t, h, "/static/css/app.css").Header().Get("Cache-Control"); strings.Contains(cc, "immutable") || cc == "" {
		t.Errorf("unversioned asset Cache-Control %q, want a short lifetime", cc)
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

// Nothing rendered carries a style attribute or a <style> block: the policy
// says `style-src 'self'` and a single inline style would be the first thing
// a browser refused. Exercised with a link that has a PIN, tags and a folder,
// which is where the last inline styles lived.
func TestNoInlineStylesAnywhere(t *testing.T) {
	h, _, _, done := setupTestServer(t)
	defer done()
	post := func(path, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		return rr
	}
	if rr := post("/api/folders", `{"name":"Launch","color":"#f59e0b"}`); rr.Code >= 300 {
		t.Fatalf("folder: %d %s", rr.Code, rr.Body.String())
	}
	if rr := post("/api/links", `{"url":"https://example.com/p?utm_source=x&id=7","custom_slug":"styled","pin":"1234","tags":["a","b"],"ios_url":"https://apps.apple.com/x"}`); rr.Code != http.StatusCreated {
		t.Fatalf("link: %d %s", rr.Code, rr.Body.String())
	}
	for _, path := range []string{"/", "/settings", "/preview/styled", "/pin/styled", "/no-such-link-here"} {
		body := get(t, h, path).Body.String()
		if strings.Contains(body, `style="`) || strings.Contains(body, "<style") {
			t.Errorf("%s renders an inline style", path)
		}
	}
	csp := get(t, h, "/").Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "style-src 'self';") || strings.Contains(csp, "unsafe-inline") {
		t.Errorf("CSP %q, want style-src 'self' with no unsafe-inline", csp)
	}
}
