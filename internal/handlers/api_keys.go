package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kaicorplabs/linkup/internal/models"
	"github.com/kaicorplabs/linkup/internal/services"
)

type APIKeyHandler struct {
	apiKeyService *services.APIKeyService
	authService   *services.AuthService
}

func NewAPIKeyHandler(apiKeyService *services.APIKeyService, authService *services.AuthService) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService: apiKeyService,
		authService:   authService,
	}
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	keys, err := h.apiKeyService.List(session.Username, session.IsAdmin)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"api_keys": keys})
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var req models.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	apiKey, rawSecret, err := h.apiKeyService.Create(req.Name, session.Username)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusCreated, models.APIKeyCreatedResponse{
		APIKey: *apiKey,
		Secret: rawSecret,
	})
}

func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.apiKeyService.Delete(id, session.Username, session.IsAdmin); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}
