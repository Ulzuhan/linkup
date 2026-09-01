package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kaicorplabs/linkup/internal/services"
)

type BulkHandler struct {
	csvService    *services.CSVService
	authService   *services.AuthService
	apiKeyService *services.APIKeyService
}

func NewBulkHandler(csvService *services.CSVService, authService *services.AuthService, apiKeyService *services.APIKeyService) *BulkHandler {
	return &BulkHandler{
		csvService:    csvService,
		authService:   authService,
		apiKeyService: apiKeyService,
	}
}

// BulkImport handles POST /api/links/bulk-import
func (h *BulkHandler) BulkImport(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Limit upload size to 10MB
	_ = r.ParseMultipartForm(10 << 20)

	file, _, err := r.FormFile("file")
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read uploaded 'file' form field"})
		return
	}
	defer file.Close()

	result, err := h.csvService.ImportCSV(file, session.Username)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusOK, result)
}

// ExportCSV handles GET /api/links/export
func (h *BulkHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	session := getAuthSession(r, h.authService, h.apiKeyService)
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	filename := fmt.Sprintf("linkup_export_%s_%s.csv", session.Username, time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if err := h.csvService.ExportCSV(w, session.Username, session.IsAdmin); err != nil {
		http.Error(w, "Export failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
