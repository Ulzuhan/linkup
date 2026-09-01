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

func TestCustomDomainsAndMultiDomainResolution(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "linkup-domain-test-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "domains.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	cache := services.NewLinkCache(1000, 5*time.Minute)
	webhooks := services.NewWebhookService(db)
	linkService := services.NewLinkService(db, cache, webhooks, "link.kaicorplabs.com")
	domainService := services.NewDomainService(db)

	// 1. Add custom domain
	cd, err := domainService.Create("go.mybrand.com", "brand-user")
	if err != nil {
		t.Fatalf("failed to create custom domain: %v", err)
	}
	if cd.Domain != "go.mybrand.com" {
		t.Errorf("expected domain 'go.mybrand.com', got %s", cd.Domain)
	}

	// 2. Create link on custom domain
	linkDomain, _, err := linkService.Create(models.CreateLinkRequest{
		URL:        "https://mybrand.com/special-deal",
		CustomSlug: "deal",
		Domain:     "go.mybrand.com",
	}, "brand-user")
	if err != nil {
		t.Fatalf("failed to create link on custom domain: %v", err)
	}

	// 3. Create same slug on default domain pointing to a different URL
	linkDefault, _, err := linkService.Create(models.CreateLinkRequest{
		URL:        "https://kaicorplabs.com/general-deal",
		CustomSlug: "deal",
		Domain:     "",
	}, "kaicorp-user")
	if err != nil {
		t.Fatalf("failed to create link on default domain: %v", err)
	}

	// 4. Resolve slug on go.mybrand.com
	resDomain, err := linkService.Resolve("go.mybrand.com", "deal")
	if err != nil {
		t.Fatalf("failed to resolve custom domain slug: %v", err)
	}
	if resDomain.TargetURL != linkDomain.TargetURL {
		t.Errorf("expected custom domain target URL %s, got %s", linkDomain.TargetURL, resDomain.TargetURL)
	}

	// 5. Resolve slug on default domain
	resDefault, err := linkService.Resolve("", "deal")
	if err != nil {
		t.Fatalf("failed to resolve default domain slug: %v", err)
	}
	if resDefault.TargetURL != linkDefault.TargetURL {
		t.Errorf("expected default domain target URL %s, got %s", linkDefault.TargetURL, resDefault.TargetURL)
	}

	// 6. List and Delete domain
	domains, err := domainService.List("brand-user", false)
	if err != nil || len(domains) != 1 {
		t.Errorf("expected 1 domain for user, got %d", len(domains))
	}

	if err := domainService.Delete(cd.ID, "brand-user", false); err != nil {
		t.Errorf("failed to delete domain: %v", err)
	}
}
