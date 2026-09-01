package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
)

func TestWebhookDispatchAndHMACSignature(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "linkup-webhook-test-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "wh.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	webhookSecret := "super-secret-webhook-key"
	receivedEvents := make(chan models.WebhookPayload, 5)
	receivedSignatures := make(chan string, 5)
	receivedBodies := make(chan []byte, 5)

	// Mock Webhook Receiver Server
	mockReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig := r.Header.Get("X-LinkUp-Signature")
		var payload models.WebhookPayload
		_ = json.Unmarshal(body, &payload)

		receivedBodies <- body
		receivedSignatures <- sig
		receivedEvents <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer mockReceiver.Close()

	cache := services.NewLinkCache(1000, 5*time.Minute)
	webhookService := services.NewWebhookService(db)
	// httptest always binds loopback, which the real destination check refuses
	// on purpose. This test is about the signature and the dispatch, so it opts
	// out explicitly; TestWebhookRefusesReservedTargets covers the check itself.
	webhookService.AllowReservedTargetsForTesting()
	linkService := services.NewLinkService(db, cache, webhookService, "link.kaicorplabs.com")

	// 1. Register Webhook
	_, err = webhookService.Create(models.CreateWebhookRequest{
		URL:    mockReceiver.URL,
		Secret: webhookSecret,
		Events: []string{"link.created", "link.self_destructed"},
	}, "webhook-user")
	if err != nil {
		t.Fatalf("failed to register webhook: %v", err)
	}

	// 2. Create link to trigger 'link.created' event
	_, _, err = linkService.Create(models.CreateLinkRequest{
		URL:        "https://example.com/webhook-trigger",
		CustomSlug: "wh-link",
	}, "webhook-user")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// 3. Wait for webhook delivery
	select {
	case payload := <-receivedEvents:
		if payload.Event != "link.created" {
			t.Errorf("expected event 'link.created', got %s", payload.Event)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for webhook event delivery")
	}

	// 4. Verify HMAC-SHA256 signature
	body := <-receivedBodies
	sigHeader := <-receivedSignatures

	expectedSig := "sha256=" + computeTestHMAC(body, webhookSecret)
	if sigHeader != expectedSig {
		t.Errorf("expected signature %s, got %s", expectedSig, sigHeader)
	}
}

func computeTestHMAC(message []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	return strings.ToLower(hex.EncodeToString(mac.Sum(nil)))
}
