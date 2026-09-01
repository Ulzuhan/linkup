package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/Ulzuhan/linkup/internal/services"
)

func TestSmartDeviceRouting(t *testing.T) {
	engine := services.NewRouterEngine()

	link := &models.Link{
		TargetURL:  "https://example.com/desktop-landing",
		IOSURL:     "https://apps.apple.com/app/id123456",
		AndroidURL: "https://play.google.com/store/apps/details?id=com.example.app",
	}

	// 1. iPhone User-Agent
	reqIOS, _ := http.NewRequest("GET", "/test", nil)
	reqIOS.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15")
	urlIOS, _ := engine.ResolveDestination(reqIOS, link)
	if urlIOS != link.IOSURL {
		t.Errorf("expected iOS App Store destination, got %s", urlIOS)
	}

	// 2. Android User-Agent
	reqAndroid, _ := http.NewRequest("GET", "/test", nil)
	reqAndroid.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36")
	urlAndroid, _ := engine.ResolveDestination(reqAndroid, link)
	if urlAndroid != link.AndroidURL {
		t.Errorf("expected Android Play Store destination, got %s", urlAndroid)
	}

	// 3. Desktop Mac/Chrome User-Agent
	reqDesktop, _ := http.NewRequest("GET", "/test", nil)
	reqDesktop.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	urlDesktop, _ := engine.ResolveDestination(reqDesktop, link)
	if urlDesktop != link.TargetURL {
		t.Errorf("expected Desktop fallback destination, got %s", urlDesktop)
	}
}

func TestSmartLocaleRouting(t *testing.T) {
	engine := services.NewRouterEngine()

	link := &models.Link{
		TargetURL: "https://example.com/en/docs",
		LocaleRouting: map[string]string{
			"es": "https://example.com/es/docs",
			"fr": "https://example.com/fr/docs",
			"de": "https://example.com/de/docs",
		},
	}

	// 1. Spanish Accept-Language
	reqES, _ := http.NewRequest("GET", "/docs", nil)
	reqES.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
	urlES, _ := engine.ResolveDestination(reqES, link)
	if urlES != "https://example.com/es/docs" {
		t.Errorf("expected Spanish URL, got %s", urlES)
	}

	// 2. French Accept-Language
	reqFR, _ := http.NewRequest("GET", "/docs", nil)
	reqFR.Header.Set("Accept-Language", "fr-FR,fr;q=0.9")
	urlFR, _ := engine.ResolveDestination(reqFR, link)
	if urlFR != "https://example.com/fr/docs" {
		t.Errorf("expected French URL, got %s", urlFR)
	}

	// 3. Japanese Accept-Language (fallback)
	reqJA, _ := http.NewRequest("GET", "/docs", nil)
	reqJA.Header.Set("Accept-Language", "ja-JP,ja;q=0.9")
	urlJA, _ := engine.ResolveDestination(reqJA, link)
	if urlJA != "https://example.com/en/docs" {
		t.Errorf("expected fallback English URL, got %s", urlJA)
	}
}

func TestABTestingDistribution(t *testing.T) {
	engine := services.NewRouterEngine()

	link := &models.Link{
		TargetURL: "https://example.com/default",
		ABVariants: []models.ABVariant{
			{Name: "Variant A", TargetURL: "https://example.com/landing-a", Weight: 50},
			{Name: "Variant B", TargetURL: "https://example.com/landing-b", Weight: 50},
		},
	}

	req := httptest.NewRequest("GET", "/ab-link", nil)

	counts := make(map[string]int)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		dest, varName := engine.ResolveDestination(req, link)
		if dest == "https://example.com/landing-a" {
			counts["a"]++
		} else if dest == "https://example.com/landing-b" {
			counts["b"]++
		}
		if varName == "" {
			t.Errorf("expected non-empty variant name")
		}
	}

	// Expect approximately 50/50 split (between 40% and 60%)
	if counts["a"] < 400 || counts["a"] > 600 {
		t.Errorf("unexpected distribution for Variant A: %d / %d", counts["a"], iterations)
	}
	if counts["b"] < 400 || counts["b"] > 600 {
		t.Errorf("unexpected distribution for Variant B: %d / %d", counts["b"], iterations)
	}
}
