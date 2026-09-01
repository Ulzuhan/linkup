package handlers

import (
	"log"
	"net/http"

	"github.com/kaicorplabs/linkup/internal/config"
	"github.com/kaicorplabs/linkup/internal/models"
	"github.com/kaicorplabs/linkup/internal/services"
)

type AuthHandler struct {
	cfg         *config.Config
	authService *services.AuthService
}

func NewAuthHandler(cfg *config.Config, authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		cfg:         cfg,
		authService: authService,
	}
}

// Login handles GET /auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.IsOIDCConfigured() {
		if h.cfg.DevMode {
			// Dev Mode instant session bypass
			session := &models.UserSession{
				UserID:   "dev-user-id",
				Username: "dev-user",
				Email:    "dev@kaicorplabs.com",
				IsAdmin:  true,
			}
			_ = h.authService.SetSessionCookie(w, session)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		http.Error(w, "OIDC authentication is not configured on this server.", http.StatusServiceUnavailable)
		return
	}

	authURL, err := h.authService.GetAuthURL(w)
	if err != nil {
		log.Printf("[AUTH] Failed to initiate OIDC flow: %v", err)
		http.Error(w, "Authentication provider error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// Callback handles GET /auth/callback
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.HandleCallback(r)
	if err != nil {
		log.Printf("[AUTH] OIDC callback verification failed: %v", err)
		http.Error(w, "Authentication failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.authService.SetSessionCookie(w, session); err != nil {
		log.Printf("[AUTH] Failed to set session cookie: %v", err)
		http.Error(w, "Failed to establish session", http.StatusInternalServerError)
		return
	}

	log.Printf("[AUTH] User '%s' logged in successfully via OIDC", session.Username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout handles GET /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.authService.ClearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Me handles GET /auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.GetSession(r)
	if err != nil || session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
		return
	}
	sendJSON(w, http.StatusOK, session)
}
