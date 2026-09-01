package models

import (
	"time"
)

type ABVariant struct {
	Name       string `json:"name"`
	TargetURL  string `json:"target_url"`
	Weight     int    `json:"weight"` // 1 - 100 percentage
	ClickCount int    `json:"click_count"`
}

type Link struct {
	ID            string            `json:"id"`
	Slug          string            `json:"slug"`
	Domain        string            `json:"domain"` // "" means default public host
	TargetURL     string            `json:"target_url"`
	OriginalURL   string            `json:"original_url"`
	Title         string            `json:"title"`
	FolderID      *string           `json:"folder_id,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	HasPIN        bool              `json:"has_pin"`
	PinHash       string            `json:"-"` // never exposed in JSON
	RedirectType  int               `json:"redirect_type"`
	ExpiresAt     *int64            `json:"expires_at,omitempty"`
	MaxClicks     *int              `json:"max_clicks,omitempty"`
	ClickCount    int               `json:"click_count"`
	LastClickedAt *int64            `json:"last_clicked_at,omitempty"`
	CreatedBy     string            `json:"created_by"`
	IsActive      bool              `json:"is_active"`
	
	// Smart Routing Options
	IOSURL        string            `json:"ios_url,omitempty"`
	AndroidURL    string            `json:"android_url,omitempty"`
	LocaleRouting map[string]string `json:"locale_routing,omitempty"` // e.g. {"es": "https://...", "fr": "https://..."}
	ABVariants    []ABVariant       `json:"ab_variants,omitempty"`

	CreatedAt     int64             `json:"created_at"`
	UpdatedAt     int64             `json:"updated_at"`
}

func (l *Link) IsExpired() bool {
	if !l.IsActive {
		return true
	}
	if l.ExpiresAt != nil && *l.ExpiresAt > 0 {
		if time.Now().Unix() >= *l.ExpiresAt {
			return true
		}
	}
	if l.MaxClicks != nil && *l.MaxClicks > 0 {
		if l.ClickCount >= *l.MaxClicks {
			return true
		}
	}
	return false
}

func (l *Link) ExpiryReason() string {
	if !l.IsActive {
		return "This link has been deactivated by its owner."
	}
	if l.ExpiresAt != nil && *l.ExpiresAt > 0 && time.Now().Unix() >= *l.ExpiresAt {
		return "This link has expired based on its scheduled time limit."
	}
	if l.MaxClicks != nil && *l.MaxClicks > 0 && l.ClickCount >= *l.MaxClicks {
		return "This link has self-destructed after reaching its maximum click budget."
	}
	return ""
}

type CreateLinkRequest struct {
	URL            string            `json:"url"`
	CustomSlug     string            `json:"custom_slug,omitempty"`
	Domain         string            `json:"domain,omitempty"`
	Title          string            `json:"title,omitempty"`
	FolderID       *string           `json:"folder_id,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	PIN            string            `json:"pin,omitempty"`
	RedirectType   int               `json:"redirect_type,omitempty"` // 301 or 302 (default: 302)
	ExpiresInHours *int              `json:"expires_in_hours,omitempty"`
	ExpiresAt      *int64            `json:"expires_at,omitempty"`
	MaxClicks      *int              `json:"max_clicks,omitempty"`
	
	// Conditional Routing fields
	IOSURL         string            `json:"ios_url,omitempty"`
	AndroidURL     string            `json:"android_url,omitempty"`
	LocaleRouting  map[string]string `json:"locale_routing,omitempty"`
	ABVariants     []ABVariant       `json:"ab_variants,omitempty"`
}

type UpdateLinkRequest struct {
	TargetURL     *string           `json:"target_url,omitempty"`
	Title         *string           `json:"title,omitempty"`
	FolderID      *string           `json:"folder_id,omitempty"`
	Tags          *[]string         `json:"tags,omitempty"`
	PIN           *string           `json:"pin,omitempty"` // "" means remove PIN, non-empty means update
	ExpiresAt     *int64            `json:"expires_at,omitempty"`
	MaxClicks     *int              `json:"max_clicks,omitempty"`
	IsActive      *bool             `json:"is_active,omitempty"`
	IOSURL        *string           `json:"ios_url,omitempty"`
	AndroidURL    *string           `json:"android_url,omitempty"`
	LocaleRouting *map[string]string `json:"locale_routing,omitempty"`
	ABVariants    *[]ABVariant       `json:"ab_variants,omitempty"`
}

type CleanPreviewResult struct {
	OriginalURL    string   `json:"original_url"`
	CleanURL       string   `json:"clean_url"`
	StrippedParams []string `json:"stripped_params"`
	IsSafe         bool     `json:"is_safe"`
	Warning        string   `json:"warning,omitempty"`
}

type UserSession struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	IsAdmin   bool   `json:"is_admin"`
	CreatedAt int64  `json:"created_at"`
}

type BlockedDomain struct {
	ID        int    `json:"id"`
	Domain    string `json:"domain"`
	Reason    string `json:"reason"`
	CreatedAt int64  `json:"created_at"`
}

type PublicPreviewData struct {
	Slug           string
	CleanURL       string
	OriginalURL    string
	StrippedParams []string
	Title          string
	HasPIN         bool
	IsExpired      bool
	ExpiryReason   string
	QRForgeLink    string
	BaseDomain     string
	CreatedAgo     string
}

type DashboardData struct {
	User          UserSession
	Links         []Link
	Folders       []Folder
	CustomDomains []CustomDomain
	CurrentFolder string
	CurrentTag    string
	TotalLinks    int
	TotalClicks   int
	PublicHost    string
	DefaultDomain string
	QRForgeURL    string
	AccountURL    string
	EnrollURL     string
	IsAdmin       bool
	FlashSuccess  string
	FlashError    string
}

type SettingsData struct {
	User          UserSession
	APIKeys       []APIKey
	CustomDomains []CustomDomain
	Webhooks      []Webhook
	PublicHost    string
	DefaultDomain string
	QRForgeURL    string
	AccountURL    string
	EnrollURL     string
	IsAdmin       bool
	FlashSuccess  string
	FlashError    string
	NewAPIKey     string // Temporary display for newly created key
}
