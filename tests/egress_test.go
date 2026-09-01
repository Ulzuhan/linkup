package tests

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
)

// The regression test for the server-side request forgery reproduced on
// 2026-09-01: a webhook pointing at loopback or at the cloud metadata address
// was accepted with 201, and creating a link made the server issue that
// request. Both halves are covered here — it must be refused when stored, and
// nothing must arrive when an event fires.

func TestWebhookRefusesReservedTargets(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "linkup-egress-test-*")
	defer os.RemoveAll(tmpDir)

	db, err := database.Open(filepath.Join(tmpDir, "egress.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	webhooks := services.NewWebhookService(db)

	for _, target := range []string{
		"http://127.0.0.1:9100/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/hook",
		"http://localhost:3464/hook",
		"ftp://example.com/hook",
	} {
		if _, err := webhooks.Create(models.CreateWebhookRequest{
			URL:    target,
			Events: []string{"*"},
		}, "attacker"); err == nil {
			t.Errorf("a webhook to %s must be refused when it is stored", target)
		}
	}
}

func TestWebhookDeliveryNeverReachesAReservedTarget(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "linkup-egress-delivery-*")
	defer os.RemoveAll(tmpDir)

	db, err := database.Open(filepath.Join(tmpDir, "egress.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	// A receiver on loopback, which is exactly what an SSRF would aim at.
	reached := make(chan struct{}, 1)
	receiver := httptest.NewServer(nethttpHandler(func() { reached <- struct{}{} }))
	defer receiver.Close()

	webhooks := services.NewWebhookService(db)
	// Stored through the relaxed path, so the row exists exactly as it would
	// have before the fix. What must hold is that DELIVERY still refuses it:
	// validating only on the way in would leave DNS rebinding open.
	webhooks.AllowReservedTargetsForTesting()
	if _, err := webhooks.Create(models.CreateWebhookRequest{
		URL:    receiver.URL + "/interno",
		Events: []string{"*"},
	}, "attacker"); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Now put the real check back and fire the event.
	strict := services.NewWebhookService(db)
	cache := services.NewLinkCache(100, time.Minute)
	links := services.NewLinkService(db, cache, strict, "link.kaicorplabs.com")
	if _, _, err := links.Create(models.CreateLinkRequest{
		URL:        "https://example.com/trigger",
		CustomSlug: "egress-trigger",
	}, "attacker"); err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	select {
	case <-reached:
		t.Fatal("the server delivered to a reserved address: the SSRF is open")
	case <-time.After(750 * time.Millisecond):
		// Nothing arrived, which is the point.
	}
}
