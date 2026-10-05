package tests

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/handlers"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/Ulzuhan/linkup/internal/web"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type oidcKeyFixture struct {
	t                *testing.T
	db               *database.DB
	router           http.Handler
	mu               sync.Mutex
	nonce            string
	status           int
	badSubject       bool
	groups           []string
	entered, release chan struct{}
}

func newOIDCKeyFixture(t *testing.T) *oidcKeyFixture {
	t.Helper()
	f := &oidcKeyFixture{t: t, status: 200, groups: []string{"linkup", "admins"}}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signed := func(claims map[string]any) string {
		header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test"})
		body, _ := json.Marshal(claims)
		input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
		hash := sha256.Sum256([]byte(input))
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		return input + "." + base64.RawURLEncoding.EncodeToString(sig)
	}
	var provider *httptest.Server
	provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "userinfo_endpoint": provider.URL + "/userinfo", "jwks_uri": provider.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			r.ParseForm()
			name := r.Form.Get("code")
			now := time.Now().Unix()
			f.mu.Lock()
			nonce := f.nonce
			groups := append([]string{}, f.groups...)
			f.mu.Unlock()
			id := signed(map[string]any{"iss": provider.URL, "aud": "linkup-client", "sub": "subject-" + name, "preferred_username": name, "sid": "sid-" + name, "nonce": nonce, "groups": groups, "iat": now, "exp": now + 3600})
			json.NewEncoder(w).Encode(map[string]any{"access_token": "access-" + name, "token_type": "Bearer", "expires_in": 3600, "id_token": id})
		case "/userinfo":
			name := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer access-")
			f.mu.Lock()
			status := f.status
			bad := f.badSubject
			groups := append([]string{}, f.groups...)
			entered, release := f.entered, f.release
			f.mu.Unlock()
			if entered != nil {
				close(entered)
				<-release
			}
			sub := "subject-" + name
			if bad {
				sub = "different-owner"
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]any{"sub": sub, "groups": groups})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	f.db, err = database.Open(filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	cfg := &config.Config{PublicHost: "link.test", DefaultDomain: "link.test", SessionSecret: []byte("synthetic-only-at-least-thirty-two-bytes"), OIDCIssuerURL: provider.URL, OIDCClientID: "linkup-client", OIDCClientSecret: "synthetic", OIDCRedirectURI: "https://link.test/auth/callback", RequiredGroup: "linkup", AdminGroup: "admins"}
	webhooks := services.NewWebhookService(f.db)
	links := services.NewLinkService(f.db, services.NewLinkCache(100, time.Minute), webhooks, cfg.PublicHost)
	renderer, err := web.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	f.router = handlers.NewRouter(cfg, f.db, links, services.NewDomainService(f.db), services.NewFolderService(f.db), services.NewAPIKeyService(f.db, func(name string) bool { return cfg.IsAdmin(name, nil) }), webhooks, services.NewCSVService(links), services.NewRouterEngine(), services.NewAuthService(cfg, f.db), renderer)
	return f
}

func (f *oidcKeyFixture) call(method, path, secret string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	out := httptest.NewRecorder()
	f.router.ServeHTTP(out, req)
	return out
}
func (f *oidcKeyFixture) login(name string) *http.Cookie {
	f.t.Helper()
	start := f.call("GET", "/auth/login", "", nil, nil)
	if start.Code != 303 {
		f.t.Fatal("login route failed")
	}
	location, _ := url.Parse(start.Header().Get("Location"))
	f.mu.Lock()
	f.nonce = location.Query().Get("nonce")
	f.mu.Unlock()
	result := f.call("GET", "/auth/callback?code="+name+"&state="+url.QueryEscape(location.Query().Get("state")), "", start.Result().Cookies()[0], nil)
	if result.Code != 303 {
		f.t.Fatalf("callback failed: %d", result.Code)
	}
	for _, cookie := range result.Result().Cookies() {
		if cookie.Name == services.SessionCookieName {
			return cookie
		}
	}
	f.t.Fatal("no session cookie")
	return nil
}
func (f *oidcKeyFixture) newKey(cookie *http.Cookie) models.APIKeyCreatedResponse {
	f.t.Helper()
	out := f.call("POST", "/api/keys", "", cookie, map[string]string{"name": "route-created"})
	if out.Code != 201 {
		f.t.Fatalf("key route failed: %d", out.Code)
	}
	var key models.APIKeyCreatedResponse
	if json.Unmarshal(out.Body.Bytes(), &key) != nil || key.Secret == "" {
		f.t.Fatal("missing key")
	}
	return key
}
func (f *oidcKeyFixture) newLink(secret, slug string) models.Link {
	f.t.Helper()
	out := f.call("POST", "/api/links", secret, nil, map[string]string{"url": "https://example.org/" + slug, "custom_slug": slug})
	if out.Code != 201 {
		f.t.Fatalf("key could not create link: %d", out.Code)
	}
	var result struct {
		Link models.Link `json:"link"`
	}
	if json.Unmarshal(out.Body.Bytes(), &result) != nil {
		f.t.Fatal("invalid link")
	}
	return result.Link
}

func TestOIDCNewKeysUseSubjectAndKeepOwnersIsolated(t *testing.T) {
	f := newOIDCKeyFixture(t)
	alice := f.newKey(f.login("alice"))
	bob := f.newKey(f.login("bob"))
	if alice.APIKey.UserID != "subject-alice" || bob.APIKey.UserID != "subject-bob" {
		t.Fatal("new key not owned by subject")
	}
	a := f.newLink(alice.Secret, "alice-key")
	b := f.newLink(bob.Secret, "bob-key")
	if a.CreatedBy != "subject-alice" || b.CreatedBy != "subject-bob" || a.CreatedAt < 1_000_000_000 || a.CreatedAt > 10_000_000_000 {
		t.Fatal("key changed owner or timestamp units")
	}
	listed := f.call("GET", "/api/links", alice.Secret, nil, nil)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), a.ID) || strings.Contains(listed.Body.String(), b.ID) {
		t.Fatal("key link list crossed owners")
	}
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		result := f.call(method, "/api/links/"+b.ID, alice.Secret, nil, map[string]any{"target_url": "https://example.org/forbidden"})
		if result.Code == 200 {
			t.Fatal("key accessed or mutated another owner")
		}
	}
	keys := f.call("GET", "/api/keys", alice.Secret, nil, nil)
	if keys.Code != 200 || !strings.Contains(keys.Body.String(), alice.APIKey.ID) || strings.Contains(keys.Body.String(), bob.APIKey.ID) {
		t.Fatal("key gained group administration")
	}
	if f.call("DELETE", "/api/keys/"+bob.APIKey.ID, alice.Secret, nil, nil).Code == 200 {
		t.Fatal("key revoked another owner's key")
	}
	if f.call("DELETE", "/api/keys/"+alice.APIKey.ID, alice.Secret, nil, nil).Code != 200 {
		t.Fatal("owner cannot revoke its key")
	}
	if f.call("GET", "/api/links", alice.Secret, nil, nil).Code != 401 {
		t.Fatal("revoked key accepted")
	}
}

func TestOIDCKeysFailClosedOnProviderSessionAndNamespaceChanges(t *testing.T) {
	for _, failure := range []string{"groups", "provider", "token", "subject", "expired", "missing"} {
		t.Run(failure, func(t *testing.T) {
			f := newOIDCKeyFixture(t)
			key := f.newKey(f.login("alice"))
			f.mu.Lock()
			switch failure {
			case "groups":
				f.groups = nil
			case "provider":
				f.status = 503
			case "token":
				f.status = 401
			case "subject":
				f.badSubject = true
			}
			f.mu.Unlock()
			switch failure {
			case "expired":
				if _, err := f.db.Exec(`UPDATE oidc_sessions SET expires_at=1`); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if _, err := f.db.Exec(`DELETE FROM oidc_sessions`); err != nil {
					t.Fatal(err)
				}
			case "ambiguous":
				if _, err := f.db.Exec(`INSERT INTO oidc_sessions SELECT 'collision','other-subject','other-sid','subject-alice',access_token,expires_at FROM oidc_sessions LIMIT 1`); err != nil {
					t.Fatal(err)
				}
			}
			if f.call("GET", "/api/links", key.Secret, nil, nil).Code != 401 {
				t.Fatal("unauthorized key accepted")
			}
		})
	}
}

func TestOIDCUntypedHistoricalKeysRequireReissue(t *testing.T) {
	for _, owner := range []string{"alice", "subject-alice"} {
		t.Run(owner, func(t *testing.T) {
			f := newOIDCKeyFixture(t)
			cookie := f.login("alice")
			key := f.newKey(cookie)
			if _, err := f.db.Exec(`UPDATE api_keys SET id='historical-key',user_id=? WHERE id=?`, owner, key.APIKey.ID); err != nil {
				t.Fatal(err)
			}
			if f.call("GET", "/api/links", key.Secret, nil, nil).Code != 401 {
				t.Fatal("untyped historical key accepted")
			}
			// Existing real-login adoption retains management of legacy metadata. New
			// bearer credentials are issued only from the verified canonical session.
			cookie = f.login("alice")
			replacement := f.newKey(cookie)
			if f.call("GET", "/api/links", replacement.Secret, nil, nil).Code != 200 {
				t.Fatal("reissued subject key rejected")
			}
			if f.call("DELETE", "/api/keys/historical-key", "", cookie, nil).Code != 200 {
				t.Fatal("historical owner cannot revoke old key")
			}
		})
	}
}

func TestOIDCSubjectKeyCannotChangeOwnerWhenLoginExpires(t *testing.T) {
	f := newOIDCKeyFixture(t)
	key := f.newKey(f.login("alice"))
	if _, err := f.db.Exec(`INSERT INTO oidc_sessions SELECT 'collision','other-subject','other-sid','subject-alice',access_token,expires_at FROM oidc_sessions LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	// A different username can equal this subject without being its owner.
	if f.call("GET", "/api/links", key.Secret, nil, nil).Code != 200 {
		t.Fatal("subject key selected an alias session")
	}
	if _, err := f.db.Exec(`DELETE FROM oidc_sessions WHERE subject='subject-alice'`); err != nil {
		t.Fatal(err)
	}
	if f.call("GET", "/api/links", key.Secret, nil, nil).Code != 401 {
		t.Fatal("key reassigned to remaining alias owner")
	}
	var owner string
	if f.db.QueryRow(`SELECT user_id FROM api_keys WHERE id=?`, key.APIKey.ID).Scan(&owner) != nil || owner != "subject-alice" {
		t.Fatal("key owner was mutated")
	}
}

func TestOIDCTypedKeyOwnerSurvivesLegacyLoginMigration(t *testing.T) {
	f := newOIDCKeyFixture(t)
	key := f.newKey(f.login("alice"))
	// Complete a second real OIDC callback with a username equal to Alice's
	// subject, invoking the historical migration through the production route.
	other := f.newKey(f.login("subject-alice"))
	var owner string
	if f.db.QueryRow(`SELECT user_id FROM api_keys WHERE id=?`, key.APIKey.ID).Scan(&owner) != nil || owner != "subject-alice" {
		t.Fatal("login migration reassigned a typed key")
	}
	link := f.newLink(key.Secret, "after-name-overlap")
	if link.CreatedBy != "subject-alice" {
		t.Fatal("typed key became the other user")
	}
	result := f.call("GET", "/api/keys", key.Secret, nil, nil)
	if result.Code != 200 || strings.Contains(result.Body.String(), other.APIKey.ID) {
		t.Fatal("typed key crossed ownership after login migration")
	}
}

func TestOIDCKeyRevocationDuringUserInfoCannotBeIgnored(t *testing.T) {
	f := newOIDCKeyFixture(t)
	cookie := f.login("alice")
	key := f.newKey(cookie)
	// Another valid key must not stand in for the exact key being checked.
	f.newKey(cookie)
	entered, release := make(chan struct{}), make(chan struct{})
	f.mu.Lock()
	f.entered, f.release = entered, release
	f.mu.Unlock()
	result := make(chan int, 1)
	go func() { result <- f.call("GET", "/api/links", key.Secret, nil, nil).Code }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("UserInfo was not called")
	}
	if _, err := f.db.Exec(`DELETE FROM api_keys WHERE id=?`, key.APIKey.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case status := <-result:
		if status != 401 {
			t.Fatal("revoked exact key accepted")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("authorization did not finish")
	}
}
