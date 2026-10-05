package services

import (
	"context"
	"testing"
	"time"
)

// A successful bare UserInfo proves that the token is accepted, not that its
// embedded group claim was refreshed. This is a synthetic bound on the current
// Supabase projection, not an assertion of immediate live group revocation.
func TestBareUserInfoRetainsJWTGroupsUntilTokenOrLocalSessionIsRejected(t *testing.T) {
	for _, projection := range []string{"subject only", "explicit empty groups"} {
		t.Run(projection, func(t *testing.T) {
			f := newLiveAuthWith(t, func(f *liveAuthFixture) { f.jwtAccess = true; f.bareUserInfo = true })
			keys := NewAPIKeyService(f.db, func(string) bool { return false })
			_, secret, err := keys.CreateOIDC("synthetic projection", f.session.UserID)
			if err != nil {
				t.Fatal(err)
			}
			apiUser, err := keys.ValidateKey(secret)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.auth.AuthorizeAPIKey(context.Background(), apiUser); err != nil {
				t.Fatal(err)
			}
			if apiUser.IsAdmin {
				t.Fatal("groups elevated the API key administrator")
			}
			f.mu.Lock()
			f.groups = []string{}
			f.bareUserInfo = projection == "subject only"
			f.mu.Unlock()
			session, err := f.auth.GetSession(f.request())
			if err != nil || !session.IsAdmin {
				t.Fatalf("old JWT claim projection changed unexpectedly: %v", err)
			}
			if err := f.auth.AuthorizeAPIKey(context.Background(), apiUser); err != nil {
				t.Fatal(err)
			}
			if apiUser.IsAdmin {
				t.Fatal("JWT fallback elevated API key administration")
			}
			// A current non-empty list without access overrides the JWT fallback.
			f.mu.Lock()
			f.bareUserInfo = false
			f.groups = []string{"unrelated"}
			f.mu.Unlock()
			if err := f.auth.AuthorizeAPIKey(context.Background(), apiUser); err == nil {
				t.Fatal("fresh denial ignored")
			}
			if _, err := f.auth.GetSession(f.request()); err == nil {
				t.Fatal("fresh denial ignored by cookie")
			}
		})
	}
}

func TestJWTGroupFallbackCannotBypassProviderOrLocalSessionDenial(t *testing.T) {
	for _, scenario := range []string{"provider401", "provider403", "local expiration", "local logout"} {
		t.Run(scenario, func(t *testing.T) {
			f := newLiveAuthWith(t, func(f *liveAuthFixture) { f.jwtAccess = true; f.bareUserInfo = true })
			keys := NewAPIKeyService(f.db, func(string) bool { return false })
			_, secret, err := keys.CreateOIDC("synthetic denial", f.session.UserID)
			if err != nil {
				t.Fatal(err)
			}
			apiUser, err := keys.ValidateKey(secret)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "provider401":
				f.mu.Lock()
				f.status = 401
				f.mu.Unlock()
			case "provider403":
				f.mu.Lock()
				f.status = 403
				f.mu.Unlock()
			case "local expiration":
				if _, err := f.db.Exec(`UPDATE oidc_sessions SET expires_at = ?`, time.Now().Unix()-1); err != nil {
					t.Fatal(err)
				}
			case "local logout":
				if err := f.auth.RevokeSession(f.request()); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.auth.AuthorizeAPIKey(context.Background(), apiUser); err == nil {
				t.Fatal("key retained rejected session")
			}
			if _, err := f.auth.GetSession(f.request()); err == nil {
				t.Fatal("cookie retained rejected session")
			}
		})
	}
}
