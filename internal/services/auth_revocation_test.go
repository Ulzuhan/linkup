package services

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/models"
)

type liveAuthFixture struct {
	t       testing.TB
	key     *rsa.PrivateKey
	server  *httptest.Server
	auth    *AuthService
	db      *database.DB
	cookie  *http.Cookie
	session *models.UserSession
	nonce   string
	mu      sync.Mutex
	groups  []string
	sub     string
	status  int
}

func (f *liveAuthFixture) signed(claims map[string]any, algorithm string) string {
	f.t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": algorithm, "kid": "test-key"})
	body, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	hash := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, hash[:])
	if err != nil {
		f.t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func newLiveAuth(t testing.TB) *liveAuthFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &liveAuthFixture{t: t, key: key, groups: []string{"linkup", "admins"}, sub: "subject-1", status: 200}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": f.server.URL, "authorization_endpoint": f.server.URL + "/authorize", "token_endpoint": f.server.URL + "/token", "userinfo_endpoint": f.server.URL + "/userinfo", "jwks_uri": f.server.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			now := time.Now().Unix()
			id := f.signed(map[string]any{"iss": f.server.URL, "aud": "linkup-client", "sub": "subject-1", "sid": "sid-1", "preferred_username": "alice", "groups": []string{"linkup", "admins"}, "nonce": f.nonce, "iat": now, "exp": now + 3600}, "RS256")
			json.NewEncoder(w).Encode(map[string]any{"access_token": "secret-access-token", "token_type": "Bearer", "expires_in": 3600, "id_token": id})
		case "/userinfo":
			f.mu.Lock()
			defer f.mu.Unlock()
			if r.Header.Get("Authorization") != "Bearer secret-access-token" {
				w.WriteHeader(401)
				return
			}
			w.WriteHeader(f.status)
			json.NewEncoder(w).Encode(map[string]any{"sub": f.sub, "groups": f.groups})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	f.db, err = database.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	cfg := &config.Config{SessionSecret: []byte("test-secret-at-least-thirty-two-bytes"), OIDCIssuerURL: f.server.URL, OIDCClientID: "linkup-client", OIDCClientSecret: "client-secret", OIDCRedirectURI: "https://link.test/auth/callback", RequiredGroup: "linkup", AdminGroup: "admins"}
	f.auth = NewAuthService(cfg, f.db)
	start := httptest.NewRecorder()
	location, err := f.auth.GetAuthURL(start)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(location)
	f.nonce = u.Query().Get("nonce")
	req := httptest.NewRequest("GET", "/auth/callback?code=test-code&state="+url.QueryEscape(u.Query().Get("state")), nil)
	req.AddCookie(start.Result().Cookies()[0])
	f.session, err = f.auth.HandleCallback(req)
	if err != nil {
		t.Fatal(err)
	}
	issued := httptest.NewRecorder()
	if err := f.auth.SetSessionCookie(issued, f.session); err != nil {
		t.Fatal(err)
	}
	f.cookie = issued.Result().Cookies()[0]
	return f
}

func (f *liveAuthFixture) request() *http.Request {
	r := httptest.NewRequest("GET", "/auth/me", nil)
	r.AddCookie(f.cookie)
	return r
}

func (f *liveAuthFixture) logoutClaims() map[string]any {
	now := time.Now().Unix()
	return map[string]any{"iss": f.server.URL, "aud": "linkup-client", "sub": "subject-1", "sid": "sid-1", "iat": now, "exp": now + 300, "jti": "logout-1", "events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}}}
}

func TestOIDCGroupRemovalAppliesOnNextRequest(t *testing.T) {
	f := newLiveAuth(t)
	session, err := f.auth.GetSession(f.request())
	if err != nil || !session.IsAdmin {
		t.Fatalf("initial login: %v", err)
	}
	apiUser := &models.UserSession{Username: "alice"}
	if err := f.auth.AuthorizeAPIKey(context.Background(), apiUser); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.groups = []string{"linkup"}
	f.mu.Unlock()
	session, err = f.auth.GetSession(f.request())
	if err != nil || session.IsAdmin {
		t.Fatalf("removed administrator retained privilege: %v", err)
	}
	f.mu.Lock()
	f.groups = []string{}
	f.mu.Unlock()
	if err := f.auth.AuthorizeAPIKey(context.Background(), apiUser); err == nil {
		t.Fatal("API key bypassed removed service group")
	}
	if _, err := f.auth.GetSession(f.request()); err == nil {
		t.Fatal("cookie bypassed removed service group")
	}
	// Re-granting access must not revive a session already revoked locally.
	f.mu.Lock()
	f.groups = []string{"linkup", "admins"}
	f.mu.Unlock()
	if _, err := f.auth.GetSession(f.request()); err == nil {
		t.Fatal("revoked session resurrected")
	}
}

func TestOIDCAuthorizationFailsClosed(t *testing.T) {
	for _, scenario := range []string{"userinfo unavailable", "token rejected", "subject mismatch", "missing groups", "expired token", "legacy cookie", "missing row", "cancelled request"} {
		t.Run(scenario, func(t *testing.T) {
			f := newLiveAuth(t)
			req := f.request()
			f.mu.Lock()
			switch scenario {
			case "userinfo unavailable":
				f.status = 503
			case "token rejected":
				f.status = 401
			case "subject mismatch":
				f.sub = "another-subject"
			case "missing groups":
				f.groups = nil
			case "expired token":
				if _, err := f.db.Exec(`UPDATE oidc_sessions SET expires_at = ?`, time.Now().Unix()-1); err != nil {
					t.Fatal(err)
				}
			case "legacy cookie":
				f.session.SessionID = ""
				rr := httptest.NewRecorder()
				if err := f.auth.SetSessionCookie(rr, f.session); err != nil {
					t.Fatal(err)
				}
				f.cookie = rr.Result().Cookies()[0]
				req = f.request()
			case "missing row":
				if _, err := f.db.Exec(`DELETE FROM oidc_sessions`); err != nil {
					t.Fatal(err)
				}
			case "cancelled request":
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			f.mu.Unlock()
			if _, err := f.auth.GetSession(req); err == nil {
				t.Fatal("authorized without current proof")
			}
		})
	}
}

func TestOIDCSessionPersistenceAndLocalLogout(t *testing.T) {
	f := newLiveAuth(t)
	var sealed string
	if err := f.db.QueryRow(`SELECT access_token FROM oidc_sessions`).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "secret-access-token") {
		t.Fatal("plaintext token in database")
	}
	encoded, _ := json.Marshal(f.session)
	if strings.Contains(string(encoded), "secret-access-token") {
		t.Fatal("token exposed in user session")
	}
	restarted := NewAuthService(f.auth.cfg, f.db)
	if _, err := restarted.GetSession(f.request()); err != nil {
		t.Fatal(err)
	}
	if err := restarted.RevokeSession(f.request()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.GetSession(f.request()); err == nil {
		t.Fatal("copied cookie survived logout")
	}
	if err := f.auth.AuthorizeAPIKey(context.Background(), &models.UserSession{Username: "alice"}); err == nil {
		t.Fatal("API key survived without active proof")
	}
}

func TestBackchannelLogoutValidationAndDurability(t *testing.T) {
	f := newLiveAuth(t)
	for _, scenario := range []string{"issuer", "audience", "expired", "future", "old", "iat", "jti", "nonce", "nonce null", "events", "event null", "event array", "identity", "signature", "HS256", "none"} {
		t.Run(scenario, func(t *testing.T) {
			claims := f.logoutClaims()
			algorithm := "RS256"
			switch scenario {
			case "issuer":
				claims["iss"] = "https://attacker.test"
			case "audience":
				claims["aud"] = "other-client"
			case "expired":
				claims["exp"] = time.Now().Unix() - 1
			case "future":
				claims["iat"] = time.Now().Unix() + 3600
			case "old":
				claims["iat"] = time.Now().Unix() - 3600
			case "iat", "jti", "events":
				delete(claims, scenario)
			case "nonce":
				claims["nonce"] = "forbidden"
			case "nonce null":
				claims["nonce"] = nil
			case "event null":
				claims["events"] = map[string]any{"http://schemas.openid.net/event/backchannel-logout": nil}
			case "event array":
				claims["events"] = map[string]any{"http://schemas.openid.net/event/backchannel-logout": []string{}}
			case "identity":
				delete(claims, "sub")
				delete(claims, "sid")
			case "HS256", "none":
				algorithm = scenario
			}
			raw := f.signed(claims, algorithm)
			if scenario == "signature" {
				parts := strings.Split(raw, ".")
				parts[2] = "invalid"
				raw = strings.Join(parts, ".")
			}
			if err := f.auth.BackchannelLogout(context.Background(), raw); err == nil {
				t.Fatal("invalid logout accepted")
			}
			if _, err := f.auth.GetSession(f.request()); err != nil {
				t.Fatalf("invalid logout destroyed active session: %v", err)
			}
		})
	}
	// With both sub and sid present, both must match. An unrelated logout is OK.
	claims := f.logoutClaims()
	claims["sid"] = "other-sid"
	claims["jti"] = "unrelated"
	if err := f.auth.BackchannelLogout(context.Background(), f.signed(claims, "RS256")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.GetSession(f.request()); err != nil {
		t.Fatal(err)
	}
	claims = f.logoutClaims()
	raw := f.signed(claims, "RS256")
	if err := f.auth.BackchannelLogout(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	restarted := NewAuthService(f.auth.cfg, f.db)
	if _, err := restarted.GetSession(f.request()); err == nil {
		t.Fatal("logout did not survive restart")
	}
	if err := restarted.BackchannelLogout(context.Background(), raw); err == nil {
		t.Fatal("replay accepted after restart")
	}
	if err := restarted.AuthorizeAPIKey(context.Background(), &models.UserSession{Username: "alice"}); err == nil {
		t.Fatal("key bypassed back-channel logout")
	}
}

func TestBackchannelLogoutRequiresExpiry(t *testing.T) {
	for _, scenario := range []string{"missing", "null", "zero", "expired", "string", "fractional"} {
		t.Run(scenario, func(t *testing.T) {
			f := newLiveAuth(t)
			claims := f.logoutClaims()
			switch scenario {
			case "missing":
				delete(claims, "exp")
			case "null":
				claims["exp"] = nil
			case "zero":
				claims["exp"] = 0
			case "expired":
				claims["exp"] = time.Now().Unix() - 1
			case "string":
				claims["exp"] = "9999999999"
			case "fractional":
				claims["exp"] = float64(time.Now().Unix()+300) + 0.5
			}
			if err := f.auth.BackchannelLogout(context.Background(), f.signed(claims, "RS256")); err == nil {
				t.Fatal("logout with missing or invalid expiration accepted")
			}
			if _, err := f.auth.GetSession(f.request()); err != nil {
				t.Fatal("invalid expiration revoked the existing session")
			}
			var count int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM oidc_logout_jtis`).Scan(&count); err != nil || count != 0 {
				t.Fatal("invalid expiration consumed the replay identifier")
			}
		})
	}
}

func TestBackchannelSubjectAndSessionSelectors(t *testing.T) {
	for _, selector := range []string{"sub", "sid"} {
		t.Run(selector, func(t *testing.T) {
			f := newLiveAuth(t)
			claims := f.logoutClaims()
			if selector == "sub" {
				delete(claims, "sid")
			} else {
				delete(claims, "sub")
			}
			if err := f.auth.BackchannelLogout(context.Background(), f.signed(claims, "RS256")); err != nil {
				t.Fatal(err)
			}
			if _, err := f.auth.GetSession(f.request()); err == nil {
				t.Fatal(fmt.Sprintf("%s logout left session active", selector))
			}
		})
	}
}
