package tests

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Ulzuhan/linkup/internal/models"
)

func TestAnonymousPreviewCannotBypassPIN(t *testing.T) {
	router, links, cleanup := setupPublicServerWithLinks(t)
	defer cleanup()
	target := "https://example.com/private-document?token=private-capability"
	_, _, err := links.Create(models.CreateLinkRequest{URL: target, CustomSlug: "protected-preview", PIN: "123456"}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/protected-preview", "/preview/protected-preview"} {
		rr := get(t, router, path)
		if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/pin/protected-preview" {
			t.Fatalf("%s: status=%d location=%s", path, rr.Code, rr.Header().Get("Location"))
		}
		if strings.Contains(rr.Body.String(), "private-capability") {
			t.Fatal("preview disclosed the destination")
		}
	}
	form := get(t, router, "/pin/protected-preview")
	if form.Code != http.StatusOK || strings.Contains(form.Body.String(), "private-capability") {
		t.Fatal("PIN form leaked or failed")
	}
	for _, tc := range []struct {
		pin    string
		status int
	}{{"wrong", 401}, {"123456", 302}} {
		req := httptest.NewRequest(http.MethodPost, "/pin/protected-preview", strings.NewReader(url.Values{"pin": {tc.pin}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != tc.status {
			t.Fatalf("PIN response=%d want=%d", rr.Code, tc.status)
		}
		if tc.status == 302 && rr.Header().Get("Location") != target {
			t.Fatal("correct PIN did not reach destination")
		}
	}
}

func TestUnprotectedPreviewStillShowsDestination(t *testing.T) {
	router, links, cleanup := setupPublicServerWithLinks(t)
	defer cleanup()
	target := "https://example.com/public-document"
	if _, _, err := links.Create(models.CreateLinkRequest{URL: target, CustomSlug: "public-preview"}, "owner"); err != nil {
		t.Fatal(err)
	}
	rr := get(t, router, "/preview/public-preview")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), target) {
		t.Fatal("public preview no longer works")
	}
}
