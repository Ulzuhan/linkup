package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaicorplabs/linkup/internal/database"
	"github.com/kaicorplabs/linkup/internal/services"
)

func TestCSVBulkImportAndExport(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "linkup-csv-test-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "csv.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	cache := services.NewLinkCache(1000, 5*time.Minute)
	webhooks := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhooks, "link.kaicorplabs.com")
	csvService := services.NewCSVService(linkService)

	// 1. Prepare sample CSV data
	csvData := `url,custom_slug,title,pin,max_clicks
https://example.com/page1?utm_source=twitter&fbclid=123,page-one,First Page,,100
https://example.com/page2?utm_medium=email,page-two,Second Page,5544,
https://example.com/page3,page-three,Third Page,,50
`
	result, err := csvService.ImportCSV(strings.NewReader(csvData), "csv-user")
	if err != nil {
		t.Fatalf("failed to import CSV: %v", err)
	}

	if result.TotalCreated != 3 {
		t.Errorf("expected 3 links created, got %d (errors: %v)", result.TotalCreated, result.Errors)
	}

	// 2. Verify links were created and cleaned
	l1, err := linkService.Resolve("", "page-one")
	if err != nil {
		t.Fatalf("failed to resolve page-one: %v", err)
	}
	if l1.TargetURL != "https://example.com/page1" {
		t.Errorf("expected clean URL without utm/fbclid, got %s", l1.TargetURL)
	}

	l2, err := linkService.Resolve("", "page-two")
	if err != nil {
		t.Fatalf("failed to resolve page-two: %v", err)
	}
	if !l2.HasPIN {
		t.Errorf("expected page-two to have PIN")
	}

	// 3. Test CSV Export
	var buf bytes.Buffer
	if err := csvService.ExportCSV(&buf, "csv-user", false); err != nil {
		t.Fatalf("failed to export CSV: %v", err)
	}

	exported := buf.String()
	if !strings.Contains(exported, "Slug,Domain,Target URL") {
		t.Errorf("expected CSV headers in export")
	}
	if !strings.Contains(exported, "page-one") || !strings.Contains(exported, "page-two") {
		t.Errorf("expected exported links in CSV, got: %s", exported)
	}
}
