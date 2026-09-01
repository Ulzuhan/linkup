package tests

import (
	"net/url"
	"testing"

	"github.com/kaicorplabs/linkup/internal/services"
)

func TestCleanerStripping(t *testing.T) {
	tests := []struct {
		name          string
		rawURL        string
		expectedClean string
		expectParams  []string
	}{
		{
			name:          "Google Analytics UTM parameters",
			rawURL:        "https://example.com/blog/article?utm_source=twitter&utm_medium=social&utm_campaign=launch&utm_content=hero&id=123",
			expectedClean: "https://example.com/blog/article?id=123",
			expectParams:  []string{"utm_campaign", "utm_content", "utm_medium", "utm_source"},
		},
		{
			name:          "Meta Facebook fbclid and igshid",
			rawURL:        "https://store.example.com/item?fbclid=IwAR3xyz987&igshid=abc1234&product=shoes",
			expectedClean: "https://store.example.com/item?product=shoes",
			expectParams:  []string{"fbclid", "igshid"},
		},
		{
			name:          "Google Ads gclid and gad_source",
			rawURL:        "https://landing.com/?gclid=CjwKCAiA...&gad_source=1&page=pricing",
			expectedClean: "https://landing.com/?page=pricing",
			expectParams:  []string{"gad_source", "gclid"},
		},
		{
			name:          "Microsoft Bing msclkid and Mailchimp mc_eid",
			rawURL:        "https://example.org/newsletter?msclkid=9999&mc_eid=abcde&subscriber=true",
			expectedClean: "https://example.org/newsletter?subscriber=true",
			expectParams:  []string{"mc_eid", "msclkid"},
		},
		{
			name:          "Spotify share tracker si",
			rawURL:        "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT?si=d263901b0b3e40be",
			expectedClean: "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT",
			expectParams:  []string{"si"},
		},
		{
			name:          "HubSpot telemetry _hsenc and _hsmi",
			rawURL:        "https://company.com/demo?_hsenc=p2ANqtz&_hsmi=12345&ref=organic",
			expectedClean: "https://company.com/demo?ref=organic",
			expectParams:  []string{"_hsenc", "_hsmi"},
		},
		{
			name:          "URL with no query params",
			rawURL:        "https://kaicorplabs.com/about",
			expectedClean: "https://kaicorplabs.com/about",
			expectParams:  nil,
		},
		{
			name:          "Missing protocol auto-prepends https",
			rawURL:        "wikipedia.org/wiki/Privacy?utm_source=test",
			expectedClean: "https://wikipedia.org/wiki/Privacy",
			expectParams:  []string{"utm_source"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanURL, stripped, err := services.CleanURL(tt.rawURL, "link.kaicorplabs.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cleanURL != tt.expectedClean {
				t.Errorf("expected clean URL %q, got %q", tt.expectedClean, cleanURL)
			}

			if len(stripped) != len(tt.expectParams) {
				t.Errorf("expected %d stripped params, got %d: %v", len(tt.expectParams), len(stripped), stripped)
			}
		})
	}
}

func TestCleanerSecurityRejections(t *testing.T) {
	invalidURLs := []struct {
		name   string
		rawURL string
	}{
		{"Empty URL", ""},
		{"Javascript URI scheme", "javascript:alert(1)"},
		{"Data URI scheme", "data:text/html,<script>alert(1)</script>"},
		{"File URI scheme", "file:///etc/passwd"},
		{"VBScript URI scheme", "vbscript:msgbox(1)"},
		{"Self host redirection loop", "https://link.kaicorplabs.com/infinite-loop"},
	}

	for _, tt := range invalidURLs {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := services.CleanURL(tt.rawURL, "link.kaicorplabs.com")
			if err == nil {
				t.Errorf("expected security error for %s (%s), but got nil", tt.name, tt.rawURL)
			}
		})
	}
}

func TestCleanerPreservesLegitimateParams(t *testing.T) {
	raw := "https://api.github.com/search/repositories?q=go+stars:>1000&sort=stars&order=desc&page=2"
	clean, stripped, err := services.CleanURL(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stripped) != 0 {
		t.Errorf("expected 0 stripped params, got: %v", stripped)
	}

	parsed, err := url.Parse(clean)
	if err != nil {
		t.Fatalf("failed to parse cleaned URL: %v", err)
	}

	if parsed.Query().Get("q") != "go stars:>1000" {
		t.Errorf("expected query param 'q' to be 'go stars:>1000', got: %s", parsed.Query().Get("q"))
	}
	if parsed.Query().Get("page") != "2" {
		t.Errorf("expected query param 'page' to be '2', got: %s", parsed.Query().Get("page"))
	}
}
