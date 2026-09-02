package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The regression test for the 404 that shipped.
//
// StaticFS hangs off a sub-FS already rooted at "static", so mounting it without
// StripPrefix made the FileServer look for static/css/app.css INSIDE static/ and
// answer 404 to every stylesheet and script. The service ran, the health check
// passed and the dashboard rendered — unstyled. Nothing failed loudly, which is
// why it reached production.
//
// It is tested through the real router, not against the handler on its own: the
// bug was in how the handler is MOUNTED, and a test of the handler alone would
// have passed while the site was still unstyled.

func TestStaticAssetsAreServed(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	cases := []struct {
		path        string
		contentType string
	}{
		{"/static/css/app.css", "text/css"},
		{"/static/js/app.js", "javascript"},
		{"/static/fonts/inter.woff2", "font"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200 — the site is being served without styles", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); !strings.Contains(got, tc.contentType) {
				t.Errorf("Content-Type %q does not contain %q", got, tc.contentType)
			}
			if rec.Body.Len() == 0 {
				t.Error("a 200 with an empty body is not a served asset")
			}
		})
	}
}

// Nothing in what the browser loads may reach outside this origin. It is the
// product's central promise and it broke twice in the same file: fonts from
// Google in the first line of the stylesheet, and the QR image fetched from a
// third party with the short URL inside the query string.
func TestServedAssetsCallNobody(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	forbidden := []string{"fonts.googleapis.com", "fonts.gstatic.com", "api.qrserver.com", "cdn.", "unpkg.com"}

	for _, path := range []string{"/static/css/app.css", "/static/js/app.js"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		body := rec.Body.String()

		for _, host := range forbidden {
			// The stylesheet and the script explain in comments why these are
			// gone, so only actual URLs count.
			if strings.Contains(body, "https://"+host) || strings.Contains(body, "//"+host+"/") {
				t.Errorf("%s still reaches %s from the visitor's browser", path, host)
			}
		}
	}
}

func TestSecurityHeadersTravelWithTheApplication(t *testing.T) {
	router, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	esperadas := map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Permissions-Policy":      "camera=()",
	}
	for header, fragmento := range esperadas {
		if got := rec.Header().Get(header); !strings.Contains(got, fragmento) {
			t.Errorf("%s = %q, should contain %q", header, got, fragmento)
		}
	}

	// Obsolete, ignored by current browsers, and actively harmful in the ones
	// that honoured it. CSP does this job now.
	if rec.Header().Get("X-XSS-Protection") != "" {
		t.Error("X-XSS-Protection should be gone")
	}
}
