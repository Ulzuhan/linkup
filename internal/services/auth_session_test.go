package services

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/models"
)

func TestSessionAgeIsEnforcedByServer(t *testing.T) {
	cfg := &config.Config{SessionSecret: []byte("test-session-secret-at-least-32-bytes"), AdminGroup: "admins"}
	auth := NewAuthService(cfg)
	now := time.Now().Unix()
	for _, tc := range []struct {
		name    string
		created int64
		valid   bool
	}{
		{"fresh", now, true}, {"within TTL", now - int64(SessionTTL/time.Second) + 60, true},
		{"at expiry", now - int64(SessionTTL/time.Second), false},
		{"one year old", now - 365*24*3600, false}, {"missing timestamp", 0, false},
		{"negative", -1, false}, {"future", now + 3600, false}, {"overflow", math.MaxInt64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(models.UserSession{Username: "owner", Groups: []string{"admins"}, CreatedAt: tc.created})
			if err != nil {
				t.Fatal(err)
			}
			token, err := encryptAESGCM(data, cfg.SessionSecret)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
			session, err := auth.GetSession(req)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
			if tc.valid && !session.IsAdmin {
				t.Fatal("fresh administrator lost their role")
			}
		})
	}
}

func TestSessionCookieIssuanceUsesServerTTL(t *testing.T) {
	cfg := &config.Config{SessionSecret: []byte("test-session-secret-at-least-32-bytes")}
	auth := NewAuthService(cfg)
	rr := httptest.NewRecorder()
	// Local development login omits CreatedAt. Issuance must fill it in.
	input := &models.UserSession{Username: "owner"}
	if err := auth.SetSessionCookie(rr, input); err != nil {
		t.Fatal(err)
	}
	cookie := rr.Result().Cookies()[0]
	if cookie.MaxAge != 12*3600 || !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("unexpected cookie options: %#v", cookie)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(cookie)
	session, err := auth.GetSession(req)
	if err != nil || session.CreatedAt <= 0 {
		t.Fatalf("new session invalid: %v", err)
	}
	if input.CreatedAt != 0 {
		t.Fatal("issuance mutated caller's session")
	}
	// An existing cookie must also expire across a process restart.
	restarted := NewAuthService(cfg)
	if _, err := restarted.GetSession(req); err != nil {
		t.Fatal(err)
	}
	rotated := NewAuthService(&config.Config{SessionSecret: []byte("rotated-test-session-secret-32-bytes")})
	if _, err := rotated.GetSession(req); err == nil {
		t.Fatal("key rotation did not revoke the cookie")
	}
}
