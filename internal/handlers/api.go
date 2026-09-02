package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
	"github.com/go-chi/chi/v5"
)

type APIHandler struct {
	cfg           *config.Config
	linkService   *services.LinkService
	authService   *services.AuthService
	apiKeyService *services.APIKeyService
}

func NewAPIHandler(
	cfg *config.Config,
	linkService *services.LinkService,
	authService *services.AuthService,
	apiKeyService *services.APIKeyService,
) *APIHandler {
	return &APIHandler{
		cfg:           cfg,
		linkService:   linkService,
		authService:   authService,
		apiKeyService: apiKeyService,
	}
}

// Helper to authenticate request via Bearer API key or OIDC session
func getAuthSession(r *http.Request, authService *services.AuthService, apiKeyService *services.APIKeyService) *models.UserSession {
	// 1. Check Bearer token
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if apiKeyService != nil {
			if session, err := apiKeyService.ValidateKey(token); err == nil && session != nil {
				return session
			}
		}
	}

	// 2. Check Cookie Session
	if authService != nil {
		if session, err := authService.GetSession(r); err == nil && session != nil {
			return session
		}
	}

	return nil
}

// CreateLink handles POST /api/links
func (h *APIHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var req models.CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	link, strippedParams, err := h.linkService.Create(req, session.Username)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	domainHost := h.cfg.DefaultDomain
	if link.Domain != "" {
		domainHost = link.Domain
	}
	shortURL := "https://" + domainHost + "/" + link.Slug

	sendJSON(w, http.StatusCreated, map[string]interface{}{
		"link":            link,
		"short_url":       shortURL,
		"stripped_params": strippedParams,
	})
}

// ListLinks handles GET /api/links
func (h *APIHandler) ListLinks(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	links, err := h.linkService.ListByUser(session.Username, session.IsAdmin)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch links"})
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"links": links,
		"total": len(links),
	})
}

// GetLink handles GET /api/links/{id}
func (h *APIHandler) GetLink(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	link, err := h.linkService.GetByID(id)
	if err != nil || link == nil {
		sendJSON(w, http.StatusNotFound, map[string]string{"error": "Link not found"})
		return
	}

	if !session.IsAdmin && link.CreatedBy != session.Username {
		sendJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden"})
		return
	}

	sendJSON(w, http.StatusOK, link)
}

// LinkQR handles GET /api/links/{id}/qr.svg and GET /api/links/{id}/qr.png
//
// The QR is drawn here rather than fetched from anywhere: see services/qr.go for
// why. Authorisation is the same as GetLink — a QR is a picture of a private
// destination, and handing it out would hand out the destination.
func (h *APIHandler) LinkQR(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	link, err := h.linkService.GetByID(id)
	if err != nil || link == nil {
		sendJSON(w, http.StatusNotFound, map[string]string{"error": "Link not found"})
		return
	}
	if !session.IsAdmin && link.CreatedBy != session.Username {
		sendJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden"})
		return
	}

	// Exactamente lo mismo que devuelve short_url al crear: si difiriera, el
	// código impreso llevaría a otro sitio que el enlace que se copió.
	dominio := link.Domain
	if dominio == "" {
		dominio = h.cfg.DefaultDomain
	}
	contenido := fmt.Sprintf("https://%s/%s", dominio, link.Slug)

	// Privado y no público: es el destino de alguien, no un recurso estático.
	w.Header().Set("Cache-Control", "private, max-age=3600")

	if strings.HasSuffix(r.URL.Path, ".png") {
		png, err := services.PNGDeQR(contenido, 512)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not render the QR"})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", link.Slug+".png"))
		_, _ = w.Write(png)
		return
	}

	svg, err := services.SVGDeQR(contenido)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not render the QR"})
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write(svg)
}

// UpdateLink handles PATCH /api/links/{id}
func (h *APIHandler) UpdateLink(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	var req models.UpdateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	updated, err := h.linkService.Update(id, req, session.Username, session.IsAdmin)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusOK, updated)
}

// DeleteLink handles DELETE /api/links/{id}
func (h *APIHandler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.linkService.Delete(id, session.Username, session.IsAdmin); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// CleanPreview handles POST /api/clean-preview
func (h *APIHandler) CleanPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	cleanURL, stripped, err := services.CleanURL(strings.TrimSpace(req.URL), h.cfg.PublicHost)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusOK, models.CleanPreviewResult{
		OriginalURL:    req.URL,
		CleanURL:       cleanURL,
		StrippedParams: stripped,
		IsSafe:         true,
	})
}

func sendJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
