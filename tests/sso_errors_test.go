package tests

import (
	"encoding/json"
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

func TestSSOAndSessionURLAndErrors(t *testing.T) {
	f := newOIDCKeyFixture(t)
	cookie := f.login("alice")

	// Authenticated /?sso=1 short-circuits to /.
	rr := f.call(http.MethodGet, "/?sso=1", "", cookie, nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Fatalf("/?sso=1 authed: code=%d loc=%q", rr.Code, rr.Header().Get("Location"))
	}

	// Authenticated /auth/login short-circuits to /.
	rr = f.call(http.MethodGet, "/auth/login", "", cookie, nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Fatalf("/auth/login authed: code=%d loc=%q", rr.Code, rr.Header().Get("Location"))
	}

	// Unauthenticated /?sso=1 initiates OIDC login.
	rr = f.call(http.MethodGet, "/?sso=1", "", nil, nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/auth/login" {
		t.Fatalf("/?sso=1 anon: code=%d loc=%q", rr.Code, rr.Header().Get("Location"))
	}

	// Logout redirects to /?signed_out=1.
	rr = f.call(http.MethodGet, "/auth/logout", "", cookie, nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/?signed_out=1" {
		t.Fatalf("/auth/logout: code=%d loc=%q", rr.Code, rr.Header().Get("Location"))
	}

	// API 404 returns strict JSON for GET/POST/DELETE.
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		rr = f.call(method, "/api/__nonexistent__", "", nil, nil)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s /api/__nonexistent__: code=%d", method, rr.Code)
		}
		if !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s /api/__nonexistent__: ct=%s", method, rr.Header().Get("Content-Type"))
		}
		var payload map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil || payload["error"] != "not_found" {
			t.Fatalf("%s /api/__nonexistent__: body=%q", method, rr.Body.String())
		}
	}

	// Multi-segment web 404 returns HTML with branded message and zero inline styles.
	rr = f.call(http.MethodGet, "/does/not/exist", "", nil, nil)
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("web 404: code=%d ct=%s", rr.Code, rr.Header().Get("Content-Type"))
	}
	body := rr.Body.String()
	if !strings.Contains(body, "This page does not exist") {
		t.Fatalf("web 404 missing headline: %s", body)
	}
	if strings.Contains(body, `style="`) || strings.Contains(body, "<style") {
		t.Fatalf("web 404 must not contain inline styles")
	}
}

func TestAccountSessionURLValidationAndCSP(t *testing.T) {
	newRouterWithSession := func(oidcIssuer, clientID, sessionURL string) (http.Handler, func()) {
		tmpDir, err := os.MkdirTemp("", "linkup-sso-cfg-*")
		if err != nil {
			t.Fatal(err)
		}
		db, err := database.Open(filepath.Join(tmpDir, "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		cfg := &config.Config{
			Port:              3464,
			PublicHost:        "localhost:3464",
			DefaultDomain:     "localhost:3464",
			SessionSecret:     []byte("super-secret-key-32-bytes-long!"),
			OIDCIssuerURL:     oidcIssuer,
			OIDCClientID:      clientID,
			AccountSessionURL: sessionURL,
			QRForgeURL:        "https://qr.example.test",
		}
		cache := services.NewLinkCache(100, time.Minute)
		webhooks := services.NewWebhookService(db)
		links := services.NewLinkService(db, cache, webhooks, cfg.PublicHost)
		renderer, err := web.NewRenderer()
		if err != nil {
			t.Fatal(err)
		}
		r := handlers.NewRouter(
			cfg, db, links, services.NewDomainService(db), services.NewFolderService(db),
			services.NewAPIKeyService(db, func(string) bool { return false }),
			webhooks, services.NewCSVService(links), services.NewRouterEngine(),
			services.NewAuthService(cfg, db), renderer,
		)
		return r, func() { db.Close(); os.RemoveAll(tmpDir) }
	}

	// Valid HTTPS session URL with OIDC injects attribute and CSP connect-src origin.
	h, done := newRouterWithSession("https://idp.example.invalid", "cid", "https://account.example.invalid/api/session")
	defer done()
	rr := get(t, h, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /: code=%d", rr.Code)
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' https://account.example.invalid;") {
		t.Fatalf("CSP missing session origin: %s", csp)
	}
	if !strings.Contains(csp, "style-src 'self';") || strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("CSP style-src regressed: %s", csp)
	}
	if !strings.Contains(rr.Body.String(), `data-session-url="https://account.example.invalid/api/session"`) {
		t.Fatalf("landing missing data-session-url")
	}

	// Non-HTTPS non-loopback URL is ignored.
	hBad, doneBad := newRouterWithSession("https://idp.example.invalid", "cid", "http://account.example.invalid/api/session")
	defer doneBad()
	rrBad := get(t, hBad, "/")
	if strings.Contains(rrBad.Body.String(), "data-session-url") || strings.Contains(rrBad.Header().Get("Content-Security-Policy"), "account.example.invalid") {
		t.Fatalf("insecure http session URL should be ignored")
	}

	// Without OIDC, AccountSessionURL is ignored.
	hNoOIDC, doneNoOIDC := newRouterWithSession("", "", "https://account.example.invalid/api/session")
	defer doneNoOIDC()
	rrNoOIDC := get(t, hNoOIDC, "/")
	if strings.Contains(rrNoOIDC.Body.String(), "data-session-url") || strings.Contains(rrNoOIDC.Header().Get("Content-Security-Policy"), "account.example.invalid") {
		t.Fatalf("session URL should be ignored without OIDC")
	}
}

func TestPanicRecovererJSONAndHTML(t *testing.T) {
	renderer, err := web.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{QRForgeURL: "https://qr.example.test"}
	panicHandler := handlers.Recoverer(renderer, cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rrAPI := httptest.NewRecorder()
	panicHandler.ServeHTTP(rrAPI, httptest.NewRequest(http.MethodGet, "/api/boom", nil))
	if rrAPI.Code != http.StatusInternalServerError || !strings.HasPrefix(rrAPI.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("api panic: code=%d ct=%s", rrAPI.Code, rrAPI.Header().Get("Content-Type"))
	}

	rrWeb := httptest.NewRecorder()
	panicHandler.ServeHTTP(rrWeb, httptest.NewRequest(http.MethodGet, "/web/boom", nil))
	if rrWeb.Code != http.StatusInternalServerError || !strings.Contains(rrWeb.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("web panic: code=%d ct=%s", rrWeb.Code, rrWeb.Header().Get("Content-Type"))
	}
	if !strings.Contains(rrWeb.Body.String(), "Something went wrong") {
		t.Fatalf("web panic missing recovery copy: %s", rrWeb.Body.String())
	}
}
