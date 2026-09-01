package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kaicorplabs/linkup/internal/models"
	"github.com/kaicorplabs/linkup/internal/services"
)

type FolderHandler struct {
	folderService *services.FolderService
	authService   *services.AuthService
	apiKeyService *services.APIKeyService
}

func NewFolderHandler(folderService *services.FolderService, authService *services.AuthService, apiKeyService *services.APIKeyService) *FolderHandler {
	return &FolderHandler{
		folderService: folderService,
		authService:   authService,
		apiKeyService: apiKeyService,
	}
}

func (h *FolderHandler) List(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	folders, err := h.folderService.List(session.Username, session.IsAdmin)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"folders": folders})
}

func (h *FolderHandler) Create(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var req models.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	folder, err := h.folderService.Create(req.Name, req.Color, session.Username)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusCreated, folder)
}

func (h *FolderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.folderService.Delete(id, session.Username, session.IsAdmin); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}
