package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
)

func setupTestDB(t *testing.T) (*database.DB, *services.LinkCache, *services.LinkService, func()) {
	tmpDir, err := os.MkdirTemp("", "linkup-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	cache := services.NewLinkCache(1000, 5*time.Minute)
	webhooks := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhooks, "link.kaicorplabs.com")

	cleanup := func() {
		db.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return db, cache, linkService, cleanup
}

func TestLinkCreationAndResolution(t *testing.T) {
	_, cache, linkService, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Create a link with dirty URL
	req := models.CreateLinkRequest{
		URL:        "https://news.ycombinator.com/item?id=12345&utm_source=rss&fbclid=abc",
		CustomSlug: "hn-post",
		Title:      "Hacker News Item",
	}

	link, stripped, err := linkService.Create(req, "test-user")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	if link.Slug != "hn-post" {
		t.Errorf("expected slug 'hn-post', got '%s'", link.Slug)
	}

	if link.TargetURL != "https://news.ycombinator.com/item?id=12345" {
		t.Errorf("expected clean URL, got '%s'", link.TargetURL)
	}

	if len(stripped) != 2 {
		t.Errorf("expected 2 stripped params, got %d: %v", len(stripped), stripped)
	}

	// 2. Resolve link (should hit LRU cache)
	resolved, err := linkService.Resolve("", "hn-post")
	if err != nil {
		t.Fatalf("failed to resolve link: %v", err)
	}
	if resolved.TargetURL != link.TargetURL {
		t.Errorf("expected target URL %s, got %s", link.TargetURL, resolved.TargetURL)
	}

	// 3. Verify Cache Hit
	cached, ok := cache.Get("", "hn-post")
	if !ok || cached == nil {
		t.Errorf("expected link in cache, but was not found")
	}
}

func TestLinkExpirationByTime(t *testing.T) {
	_, _, linkService, cleanup := setupTestDB(t)
	defer cleanup()

	pastTime := time.Now().Add(-1 * time.Hour).Unix()
	req := models.CreateLinkRequest{
		URL:        "https://example.com/expired-item",
		CustomSlug: "time-expired",
		ExpiresAt:  &pastTime,
	}

	_, _, err := linkService.Create(req, "test-user")
	if err != nil {
		t.Fatalf("failed to create expired link: %v", err)
	}

	// Attempt to resolve
	resolved, err := linkService.Resolve("", "time-expired")
	if err == nil {
		t.Errorf("expected error for expired link, but got nil")
	}
	if !resolved.IsExpired() {
		t.Errorf("expected IsExpired() to be true")
	}
}

func TestLinkSelfDestructByClickBudget(t *testing.T) {
	_, _, linkService, cleanup := setupTestDB(t)
	defer cleanup()

	maxClicks := 2
	req := models.CreateLinkRequest{
		URL:        "https://example.com/confidential",
		CustomSlug: "self-destruct",
		MaxClicks:  &maxClicks,
	}

	link, _, err := linkService.Create(req, "test-user")
	if err != nil {
		t.Fatalf("failed to create link with click budget: %v", err)
	}

	// 1st click
	linkService.RecordClick(link.ID, link.Domain, link.Slug, "")
	time.Sleep(50 * time.Millisecond) // wait for async click update

	// 2nd click
	linkService.RecordClick(link.ID, link.Domain, link.Slug, "")
	time.Sleep(50 * time.Millisecond)

	// Now resolving should fail as budget is reached
	resolved, err := linkService.Resolve("", "self-destruct")
	if err == nil && !resolved.IsExpired() {
		t.Errorf("expected link to be expired after reaching max clicks")
	}
}

func TestPINProtection(t *testing.T) {
	_, _, linkService, cleanup := setupTestDB(t)
	defer cleanup()

	req := models.CreateLinkRequest{
		URL:        "https://example.com/secret",
		CustomSlug: "vault",
		PIN:        "4829",
	}

	link, _, err := linkService.Create(req, "test-user")
	if err != nil {
		t.Fatalf("failed to create PIN-protected link: %v", err)
	}

	if !link.HasPIN {
		t.Errorf("expected HasPIN to be true")
	}

	// Verify PIN
	if !services.VerifyPIN("4829", link.PinHash) {
		t.Errorf("expected PIN 4829 to be valid")
	}

	if services.VerifyPIN("0000", link.PinHash) {
		t.Errorf("expected incorrect PIN to be rejected")
	}
}

// BenchmarkLRUResolution benchmarks in-memory LRU resolution speed
func BenchmarkLRUResolution(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "linkup-bench-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "bench.db")
	db, _ := database.Open(dbPath)
	defer db.Close()

	cache := services.NewLinkCache(5000, 10*time.Minute)
	webhooks := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhooks, "link.kaicorplabs.com")

	req := models.CreateLinkRequest{
		URL:        "https://example.com/fast",
		CustomSlug: "bench-link",
	}
	_, _, _ = linkService.Create(req, "bench-user")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = linkService.Resolve("", "bench-link")
	}
}
