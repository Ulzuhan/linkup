package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kaicorplabs/linkup/internal/models"
	"github.com/kaicorplabs/linkup/internal/services"
)

type DomainHandler struct {
	domainService *services.DomainService
	authService   *services.AuthService
	apiKeyService *services.APIKeyService
}

func NewDomainHandler(domainService *services.DomainService, authService *services.AuthService, apiKeyService *services.APIKeyService) *DomainHandler {
	return &DomainHandler{
		domainService: domainService,
		authService:   authService,
		apiKeyService: apiKeyService,
	}
}

func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	domains, err := h.domainService.List(session.Username, session.IsAdmin)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"domains": domains})
}

func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var req models.CreateDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	domain, err := h.domainService.Create(req.Domain, session.Username)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusCreated, domain)
}

func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.domainService.Delete(id, session.Username, session.IsAdmin); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}
