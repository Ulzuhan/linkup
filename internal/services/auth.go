package services

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	SessionCookieName = "linkup_session"
	StateCookieName   = "linkup_oidc_state"
)

type AuthService struct {
	cfg          *config.Config
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	provider     *oidc.Provider
}

func NewAuthService(cfg *config.Config) *AuthService {
	as := &AuthService{cfg: cfg}

	if cfg.IsOIDCConfigured() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
		if err != nil {
			log.Printf("[WARN] Failed to discover OIDC provider at %s: %v (will retry on login)", cfg.OIDCIssuerURL, err)
		} else {
			as.provider = provider
			as.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
			as.oauth2Config = &oauth2.Config{
				ClientID:     cfg.OIDCClientID,
				ClientSecret: cfg.OIDCClientSecret,
				RedirectURL:  cfg.OIDCRedirectURI,
				Endpoint:     provider.Endpoint(),
				// No "groups" scope is requested on purpose. Providers carry group
				// membership inside the profile claim set —Authentik's default
				// profile mapping emits it— and asking for a scope the provider
				// does not define is answered with invalid_scope, which fails the
				// whole sign-in for a claim we were already getting.
				Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
			}
			log.Printf("[AUTH] OIDC provider configured successfully for %s", cfg.OIDCIssuerURL)
		}
	} else {
		log.Println("[AUTH] OIDC not configured; running in standalone / local mode.")
	}

	return as
}

// GetAuthURL generates the Authentik OIDC redirect URL with CSRF state
func (a *AuthService) GetAuthURL(w http.ResponseWriter) (string, error) {
	if a.oauth2Config == nil {
		if err := a.reinitProvider(); err != nil {
			return "", fmt.Errorf("OIDC is not available: %w", err)
		}
	}

	state := config.GenerateRandomKey(16)
	nonce := config.GenerateRandomKey(16)

	// Store state in temporary cookie
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    state + ":" + nonce,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.cfg.DevMode,
	})

	return a.oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce)), nil
}

// HandleCallback exchanges authorization code for tokens and creates user session
func (a *AuthService) HandleCallback(r *http.Request) (*models.UserSession, error) {
	if a.oauth2Config == nil {
		return nil, errors.New("OIDC is not configured")
	}

	stateCookie, err := r.Cookie(StateCookieName)
	if err != nil {
		return nil, errors.New("missing OIDC state cookie")
	}

	parts := strings.Split(stateCookie.Value, ":")
	if len(parts) != 2 {
		return nil, errors.New("invalid state cookie format")
	}
	expectedState := parts[0]
	expectedNonce := parts[1]

	stateQuery := r.URL.Query().Get("state")
	if stateQuery == "" || stateQuery != expectedState {
		return nil, errors.New("invalid or mismatched OIDC state (possible CSRF)")
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, errors.New("missing authorization code in callback")
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	oauth2Token, err := a.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("missing id_token in token response")
	}

	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	if idToken.Nonce != expectedNonce {
		return nil, errors.New("ID token nonce does not match")
	}

	var claims struct {
		Subject           string   `json:"sub"`
		PreferredUsername string   `json:"preferred_username"`
		Email             string   `json:"email"`
		Name              string   `json:"name"`
		Groups            []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = claims.Subject
	}

	isAdmin := a.cfg.IsAdmin(username, claims.Groups) || (a.cfg.DevMode && username == "dev-user")

	return &models.UserSession{
		UserID:    claims.Subject,
		Username:  username,
		Email:     claims.Email,
		Groups:    claims.Groups,
		IsAdmin:   isAdmin,
		CreatedAt: time.Now().Unix(),
	}, nil
}

// SetSessionCookie encrypts and sets session cookie
func (a *AuthService) SetSessionCookie(w http.ResponseWriter, session *models.UserSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}

	encrypted, err := encryptAESGCM(data, a.cfg.SessionSecret)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    encrypted,
		Path:     "/",
		MaxAge:   7 * 24 * 3600, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.cfg.DevMode,
	})

	return nil
}

// ClearSessionCookie removes session cookie
func (a *AuthService) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.cfg.DevMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.cfg.DevMode,
	})
}

// GetSession reads and decrypts session from request cookie
func (a *AuthService) GetSession(r *http.Request) (*models.UserSession, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		// In DevMode without cookie, check if we should return mock user
		if a.cfg.DevMode && !a.cfg.IsOIDCConfigured() {
			return &models.UserSession{
				UserID:    "dev-user-id",
				Username:  "dev-user",
				Email:     "dev@kaicorplabs.com",
				IsAdmin:   true,
				CreatedAt: time.Now().Unix(),
			}, nil
		}
		return nil, errors.New("no session cookie found")
	}

	decrypted, err := decryptAESGCM(cookie.Value, a.cfg.SessionSecret)
	if err != nil {
		return nil, err
	}

	var session models.UserSession
	if err := json.Unmarshal(decrypted, &session); err != nil {
		return nil, err
	}

	// Re-evaluated on every read so a change of policy —which group administers—
	// takes effect without waiting for sessions to expire.
	session.IsAdmin = a.cfg.IsAdmin(session.Username, session.Groups) ||
		(a.cfg.DevMode && session.Username == "dev-user")

	return &session, nil
}

func (a *AuthService) reinitProvider() error {
	if !a.cfg.IsOIDCConfigured() {
		return errors.New("OIDC configuration is missing (LINKUP_OIDC_DISCOVERY_URL, LINKUP_OIDC_CLIENT_ID)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider, err := oidc.NewProvider(ctx, a.cfg.OIDCIssuerURL)
	if err != nil {
		return err
	}
	a.provider = provider
	a.verifier = provider.Verifier(&oidc.Config{ClientID: a.cfg.OIDCClientID})
	a.oauth2Config = &oauth2.Config{
		ClientID:     a.cfg.OIDCClientID,
		ClientSecret: a.cfg.OIDCClientSecret,
		RedirectURL:  a.cfg.OIDCRedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return nil
}

// AES-GCM helpers with SHA256 key derivation
func encryptAESGCM(plaintext, keyBytes []byte) (string, error) {
	hasher := sha256.New()
	hasher.Write(keyBytes)
	key := hasher.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func decryptAESGCM(encoded string, keyBytes []byte) ([]byte, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	hasher := sha256.New()
	hasher.Write(keyBytes)
	key := hasher.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, actualCiphertext, nil)
}
