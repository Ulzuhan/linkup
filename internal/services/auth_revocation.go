package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var errAccessRevoked = errors.New("OIDC access is no longer authorized")

func (a *AuthService) currentProvider() (*oidc.Provider, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.provider == nil {
		if err := a.reinitProvider(); err != nil {
			return nil, err
		}
	}
	return a.provider, nil
}

func (a *AuthService) authorizeWithProvider(ctx context.Context, provider *oidc.Provider, token *oauth2.Token, session *models.UserSession) error {
	// No refresh or cached-claims fallback: an expired credential requires login.
	if !token.Valid() {
		return errAccessRevoked
	}
	info, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if err != nil {
		// Do not include a provider's error body (which could contain credentials).
		return errors.New("could not verify current OIDC authorization")
	}
	if info.Subject == "" || info.Subject != session.UserID {
		return errAccessRevoked
	}
	var claims struct {
		Groups []string `json:"groups"`
	}
	if err := info.Claims(&claims); err != nil {
		return errAccessRevoked
	}
	if a.cfg.RequiredGroup != "" && !slices.Contains(claims.Groups, a.cfg.RequiredGroup) {
		return errAccessRevoked
	}
	session.Groups = claims.Groups
	return nil
}

func (a *AuthService) saveOIDCSession(ctx context.Context, session *models.UserSession, sid string, token *oauth2.Token) error {
	if a.db == nil || session.UserID == "" || token.AccessToken == "" || token.Expiry.IsZero() || !token.Valid() {
		return errors.New("cannot establish a verifiable OIDC session")
	}
	expires := min(token.Expiry.Unix(), session.CreatedAt+int64(SessionTTL/time.Second))
	sealed, err := encryptAESGCM([]byte(token.AccessToken), a.cfg.SessionSecret)
	if err != nil {
		return err
	}
	session.SessionID = config.GenerateRandomKey(32)
	// Bounded by credential lifetime. Never retain plaintext tokens in SQLite.
	if _, err := a.db.ExecContext(ctx, `DELETE FROM oidc_sessions WHERE expires_at <= ?`, time.Now().Unix()); err != nil {
		return err
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO oidc_sessions (id, subject, sid, username, access_token, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		session.SessionID, session.UserID, sid, session.Username, sealed, expires)
	return err
}

func (a *AuthService) authorizeSession(ctx context.Context, session *models.UserSession) error {
	if a.db == nil || session.SessionID == "" {
		// Also rejects every legacy cookie during the upgrade, regardless of age.
		return errAccessRevoked
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var sealed, subject, username string
	var expires int64
	if err := a.db.QueryRowContext(ctx, `SELECT subject, username, access_token, expires_at FROM oidc_sessions WHERE id = ?`, session.SessionID).
		Scan(&subject, &username, &sealed, &expires); err != nil {
		return errAccessRevoked
	}
	if subject != session.UserID || username != session.Username || expires <= time.Now().Unix() {
		return errAccessRevoked
	}
	raw, err := decryptAESGCM(sealed, a.cfg.SessionSecret)
	if err != nil {
		return errAccessRevoked
	}
	provider, err := a.currentProvider()
	if err != nil {
		return errors.New("OIDC provider unavailable")
	}
	err = a.authorizeWithProvider(ctx, provider, &oauth2.Token{AccessToken: string(raw), TokenType: "Bearer", Expiry: time.Unix(expires, 0)}, session)
	if err != nil {
		if errors.Is(err, errAccessRevoked) {
			_, _ = a.db.ExecContext(ctx, `DELETE FROM oidc_sessions WHERE id = ?`, session.SessionID)
		}
		return err
	}
	// A concurrent back-channel/local logout must not resurrect a loaded row.
	var present int
	if err := a.db.QueryRowContext(ctx, `SELECT 1 FROM oidc_sessions WHERE id = ? AND expires_at > ?`, session.SessionID, time.Now().Unix()).Scan(&present); err != nil {
		return errAccessRevoked
	}
	return nil
}

// AuthorizeAPIKey prevents a persistent key from bypassing OIDC group removal.
// In OIDC mode it needs the owner's latest unexpired login, checked live. It
// never promotes the key to group administrator. Standalone behavior is unchanged.
func (a *AuthService) AuthorizeAPIKey(ctx context.Context, session *models.UserSession) error {
	if !a.cfg.IsOIDCConfigured() {
		return nil
	}
	if a.db == nil {
		return errAccessRevoked
	}
	proof := &models.UserSession{Username: session.Username}
	if err := a.db.QueryRowContext(ctx, `SELECT id, subject FROM oidc_sessions WHERE username = ? AND expires_at > ? ORDER BY expires_at DESC LIMIT 1`, session.Username, time.Now().Unix()).
		Scan(&proof.SessionID, &proof.UserID); err != nil {
		return errAccessRevoked
	}
	return a.authorizeSession(ctx, proof)
}

// RevokeSession invalidates a copied cookie too, without depending on UserInfo.
func (a *AuthService) RevokeSession(r *http.Request) error {
	if !a.cfg.IsOIDCConfigured() {
		return nil
	}
	if a.db == nil {
		return errors.New("session store unavailable")
	}
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil
	}
	raw, err := decryptAESGCM(cookie.Value, a.cfg.SessionSecret)
	if err != nil {
		return nil
	}
	var session models.UserSession
	if json.Unmarshal(raw, &session) != nil || session.SessionID == "" {
		return nil
	}
	_, err = a.db.ExecContext(r.Context(), `DELETE FROM oidc_sessions WHERE id = ?`, session.SessionID)
	return err
}

// BackchannelLogout implements OIDC Back-Channel Logout 1.0. UserInfo remains
// the authorization guarantee even when a provider does not deliver this event.
func (a *AuthService) BackchannelLogout(ctx context.Context, raw string) error {
	if a.db == nil || raw == "" {
		return errAccessRevoked
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	provider, err := a.currentProvider()
	if err != nil {
		return err
	}
	// exp is optional for logout tokens; validate it ourselves when present.
	verifier := provider.Verifier(&oidc.Config{ClientID: a.cfg.OIDCClientID, SkipIssuerCheck: a.cfg.OIDCInternalBase != "", SkipExpiryCheck: true})
	token, err := verifier.Verify(ctx, raw)
	if err != nil || !emisorValido(token.Issuer, a.cfg.OIDCIssuerURL, a.cfg.OIDCInternalBase) {
		return errAccessRevoked
	}
	var claims struct {
		SID    string                     `json:"sid"`
		JTI    string                     `json:"jti"`
		IAT    int64                      `json:"iat"`
		EXP    *int64                     `json:"exp"`
		Events map[string]json.RawMessage `json:"events"`
	}
	var all map[string]json.RawMessage
	if token.Claims(&claims) != nil || token.Claims(&all) != nil {
		return errAccessRevoked
	}
	now := time.Now().Unix()
	_, nonce := all["nonce"]
	var event map[string]json.RawMessage
	if nonce || claims.JTI == "" || claims.IAT <= 0 || claims.IAT > now+60 || claims.IAT < now-300 ||
		(claims.EXP != nil && *claims.EXP <= now) || (token.Subject == "" && claims.SID == "") ||
		json.Unmarshal(claims.Events["http://schemas.openid.net/event/backchannel-logout"], &event) != nil || event == nil {
		return errAccessRevoked
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM oidc_logout_jtis WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO oidc_logout_jtis (jti, expires_at) VALUES (?, ?)`, claims.JTI, now+600); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM oidc_sessions WHERE (? = '' OR subject = ?) AND (? = '' OR sid = ?)`, token.Subject, token.Subject, claims.SID, claims.SID); err != nil {
		return err
	}
	return tx.Commit()
}
