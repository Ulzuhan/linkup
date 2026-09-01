package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kaicorplabs/linkup/internal/database"
	"github.com/kaicorplabs/linkup/internal/models"
)

var (
	slugRegex     = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,64}$`)
	reservedSlugs = map[string]bool{
		"api":         true,
		"auth":        true,
		"preview":     true,
		"pin":         true,
		"static":      true,
		"assets":      true,
		"dashboard":   true,
		"settings":    true,
		"health":      true,
		"healthz":     true,
		"favicon.ico": true,
		"robots.txt":  true,
		"login":       true,
		"logout":      true,
		"admin":       true,
	}
	charset = "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ" // base56
)

type LinkService struct {
	db       *database.DB
	cache    *LinkCache
	webhooks *WebhookService
	ownHost  string
}

func NewLinkService(db *database.DB, cache *LinkCache, webhooks *WebhookService, ownHost string) *LinkService {
	return &LinkService{
		db:       db,
		cache:    cache,
		webhooks: webhooks,
		ownHost:  ownHost,
	}
}

// Create creates a new shortened link with cleaned target URL and smart routing
func (s *LinkService) Create(req models.CreateLinkRequest, createdBy string) (*models.Link, []string, error) {
	// 1. Clean and sanitize URL
	cleanTarget, strippedParams, err := CleanURL(req.URL, s.ownHost)
	if err != nil {
		return nil, nil, err
	}

	// 2. Check if destination domain is blocked
	if err := s.checkDomainBlocked(cleanTarget); err != nil {
		return nil, nil, err
	}

	// 3. Clean optional iOS / Android / A-B URLs
	var cleanIOS, cleanAndroid string
	if req.IOSURL != "" {
		c, _, err := CleanURL(req.IOSURL, s.ownHost)
		if err == nil {
			cleanIOS = c
		}
	}
	if req.AndroidURL != "" {
		c, _, err := CleanURL(req.AndroidURL, s.ownHost)
		if err == nil {
			cleanAndroid = c
		}
	}

	// Sanitize A/B variants
	var sanitizedVariants []models.ABVariant
	for _, v := range req.ABVariants {
		if strings.TrimSpace(v.TargetURL) != "" {
			c, _, err := CleanURL(v.TargetURL, s.ownHost)
			if err == nil {
				v.TargetURL = c
				sanitizedVariants = append(sanitizedVariants, v)
			}
		}
	}

	domain := strings.ToLower(strings.TrimSpace(req.Domain))

	// 4. Determine slug
	slug := strings.TrimSpace(req.CustomSlug)
	if slug != "" {
		if err := s.ValidateCustomSlug(slug); err != nil {
			return nil, nil, err
		}
		// Check uniqueness within domain
		existing, err := s.GetBySlugExact(domain, slug)
		if err == nil && existing != nil {
			return nil, nil, fmt.Errorf("slug '%s' is already in use for this domain", slug)
		}
	} else {
		generatedSlug, err := s.generateUniqueSlug(domain)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate unique slug: %w", err)
		}
		slug = generatedSlug
	}

	// 5. Handle PIN
	var pinHash string
	hasPIN := false
	if strings.TrimSpace(req.PIN) != "" {
		h, err := HashPIN(strings.TrimSpace(req.PIN))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to hash PIN: %w", err)
		}
		pinHash = h
		hasPIN = true
	}

	// 6. Expiration & Click Budget
	var expiresAt *int64
	if req.ExpiresAt != nil && *req.ExpiresAt > 0 {
		expiresAt = req.ExpiresAt
	} else if req.ExpiresInHours != nil && *req.ExpiresInHours > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresInHours) * time.Hour).Unix()
		expiresAt = &t
	}

	redirectType := 302
	if req.RedirectType == 301 {
		redirectType = 301
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = slug
	}

	tagsJSON, _ := json.Marshal(req.Tags)
	localeJSON, _ := json.Marshal(req.LocaleRouting)
	variantsJSON, _ := json.Marshal(sanitizedVariants)

	now := time.Now().Unix()
	link := &models.Link{
		ID:            uuid.New().String(),
		Slug:          slug,
		Domain:        domain,
		TargetURL:     cleanTarget,
		OriginalURL:   req.URL,
		Title:         title,
		FolderID:      req.FolderID,
		Tags:          req.Tags,
		HasPIN:        hasPIN,
		PinHash:       pinHash,
		RedirectType:  redirectType,
		ExpiresAt:     expiresAt,
		MaxClicks:     req.MaxClicks,
		ClickCount:    0,
		CreatedBy:     createdBy,
		IsActive:      true,
		IOSURL:        cleanIOS,
		AndroidURL:    cleanAndroid,
		LocaleRouting: req.LocaleRouting,
		ABVariants:    sanitizedVariants,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	query := `INSERT INTO links (
		id, slug, domain, target_url, original_url, title, folder_id, tags, pin_hash, redirect_type, 
		expires_at, max_clicks, click_count, created_by, is_active, ios_url, android_url, locale_routing, ab_variants, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.Exec(query,
		link.ID, link.Slug, link.Domain, link.TargetURL, link.OriginalURL, link.Title,
		link.FolderID, string(tagsJSON), link.PinHash, link.RedirectType, link.ExpiresAt, link.MaxClicks,
		link.ClickCount, link.CreatedBy, 1, link.IOSURL, link.AndroidURL, string(localeJSON), string(variantsJSON),
		link.CreatedAt, link.UpdatedAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to save link to database: %w", err)
	}

	// Store in cache
	s.cache.Set(link.Domain, link.Slug, link)

	// Dispatch webhook
	if s.webhooks != nil {
		s.webhooks.Dispatch("link.created", link, link.CreatedBy)
	}

	return link, strippedParams, nil
}

// Resolve retrieves and validates a link by domain and slug
func (s *LinkService) Resolve(domain, slug string) (*models.Link, error) {
	// Try cache first
	if cached, ok := s.cache.Get(domain, slug); ok {
		if cached.IsExpired() {
			s.cache.Delete(domain, slug)
			return cached, fmt.Errorf("link has expired or reached click limit")
		}
		return cached, nil
	}

	// Database query
	link, err := s.GetBySlugRaw(domain, slug)
	if err != nil {
		// Fallback check root domain if queried with specific domain
		if domain != "" {
			link, err = s.GetBySlugRaw("", slug)
		}
		if err != nil {
			return nil, err
		}
	}

	if link.IsExpired() {
		return link, fmt.Errorf("link has expired or reached click limit")
	}

	s.cache.Set(link.Domain, link.Slug, link)
	return link, nil
}

// RecordClick increments click counter and records variant hits asynchronously
func (s *LinkService) RecordClick(linkID, domain, slug, variantName string) {
	go func() {
		now := time.Now().Unix()
		query := `UPDATE links SET click_count = click_count + 1, last_clicked_at = ? WHERE id = ?`
		_, _ = s.db.Exec(query, now, linkID)

		updated, err := s.GetByID(linkID)
		if err == nil && updated != nil {
			// If variant clicked, increment its count
			if variantName != "" && len(updated.ABVariants) > 0 {
				for i := range updated.ABVariants {
					if updated.ABVariants[i].Name == variantName {
						updated.ABVariants[i].ClickCount++
						break
					}
				}
				vJSON, _ := json.Marshal(updated.ABVariants)
				_, _ = s.db.Exec(`UPDATE links SET ab_variants = ? WHERE id = ?`, string(vJSON), linkID)
			}

			if updated.IsExpired() {
				s.cache.Delete(domain, slug)
				if s.webhooks != nil {
					s.webhooks.Dispatch("link.self_destructed", updated, updated.CreatedBy)
				}
			} else {
				s.cache.Set(domain, slug, updated)
			}
		}
	}()
}

func (s *LinkService) GetBySlugExact(domain, slug string) (*models.Link, error) {
	query := `SELECT id, slug, domain, target_url, original_url, title, folder_id, tags, pin_hash, redirect_type,
		expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, ios_url, android_url, locale_routing, ab_variants, created_at, updated_at
		FROM links WHERE slug = ? AND domain = ? LIMIT 1`
	return scanLink(s.db.QueryRow(query, slug, domain))
}

func (s *LinkService) GetBySlugRaw(domain, slug string) (*models.Link, error) {
	var query string
	var row *sql.Row

	if domain != "" {
		query = `SELECT id, slug, domain, target_url, original_url, title, folder_id, tags, pin_hash, redirect_type,
			expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, ios_url, android_url, locale_routing, ab_variants, created_at, updated_at
			FROM links WHERE slug = ? AND (domain = ? OR domain = '') ORDER BY domain DESC LIMIT 1`
		row = s.db.QueryRow(query, slug, domain)
	} else {
		query = `SELECT id, slug, domain, target_url, original_url, title, folder_id, tags, pin_hash, redirect_type,
			expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, ios_url, android_url, locale_routing, ab_variants, created_at, updated_at
			FROM links WHERE slug = ? AND domain = '' LIMIT 1`
		row = s.db.QueryRow(query, slug)
	}

	return scanLink(row)
}

func (s *LinkService) GetByID(id string) (*models.Link, error) {
	query := `SELECT id, slug, domain, target_url, original_url, title, folder_id, tags, pin_hash, redirect_type,
		expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, ios_url, android_url, locale_routing, ab_variants, created_at, updated_at
		FROM links WHERE id = ?`

	return scanLink(s.db.QueryRow(query, id))
}

func (s *LinkService) ListByUser(username string, isAdmin bool) ([]models.Link, error) {
	var query string
	var rows *sql.Rows
	var err error

	if isAdmin {
		query = `SELECT id, slug, domain, target_url, original_url, title, folder_id, tags, pin_hash, redirect_type,
			expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, ios_url, android_url, locale_routing, ab_variants, created_at, updated_at
			FROM links ORDER BY created_at DESC`
		rows, err = s.db.Query(query)
	} else {
		query = `SELECT id, slug, domain, target_url, original_url, title, folder_id, tags, pin_hash, redirect_type,
			expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, ios_url, android_url, locale_routing, ab_variants, created_at, updated_at
			FROM links WHERE created_by = ? ORDER BY created_at DESC`
		rows, err = s.db.Query(query, username)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanLinks(rows)
}

func (s *LinkService) Update(id string, req models.UpdateLinkRequest, username string, isAdmin bool) (*models.Link, error) {
	link, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	if !isAdmin && link.CreatedBy != username {
		return nil, fmt.Errorf("unauthorized to edit this link")
	}

	if req.TargetURL != nil {
		cleanTarget, _, err := CleanURL(*req.TargetURL, s.ownHost)
		if err != nil {
			return nil, err
		}
		if err := s.checkDomainBlocked(cleanTarget); err != nil {
			return nil, err
		}
		link.TargetURL = cleanTarget
	}

	if req.Title != nil {
		link.Title = strings.TrimSpace(*req.Title)
	}

	if req.FolderID != nil {
		link.FolderID = req.FolderID
	}

	if req.Tags != nil {
		link.Tags = *req.Tags
	}

	if req.PIN != nil {
		if *req.PIN == "" {
			link.PinHash = ""
			link.HasPIN = false
		} else {
			h, err := HashPIN(*req.PIN)
			if err != nil {
				return nil, err
			}
			link.PinHash = h
			link.HasPIN = true
		}
	}

	if req.ExpiresAt != nil {
		link.ExpiresAt = req.ExpiresAt
	}

	if req.MaxClicks != nil {
		link.MaxClicks = req.MaxClicks
	}

	if req.IsActive != nil {
		link.IsActive = *req.IsActive
	}

	if req.IOSURL != nil {
		link.IOSURL = *req.IOSURL
	}
	if req.AndroidURL != nil {
		link.AndroidURL = *req.AndroidURL
	}
	if req.LocaleRouting != nil {
		link.LocaleRouting = *req.LocaleRouting
	}
	if req.ABVariants != nil {
		link.ABVariants = *req.ABVariants
	}

	link.UpdatedAt = time.Now().Unix()

	tagsJSON, _ := json.Marshal(link.Tags)
	localeJSON, _ := json.Marshal(link.LocaleRouting)
	variantsJSON, _ := json.Marshal(link.ABVariants)

	query := `UPDATE links SET target_url = ?, title = ?, folder_id = ?, tags = ?, pin_hash = ?, 
		expires_at = ?, max_clicks = ?, is_active = ?, ios_url = ?, android_url = ?, locale_routing = ?, ab_variants = ?, updated_at = ? WHERE id = ?`
	activeInt := 0
	if link.IsActive {
		activeInt = 1
	}

	_, err = s.db.Exec(query,
		link.TargetURL, link.Title, link.FolderID, string(tagsJSON), link.PinHash,
		link.ExpiresAt, link.MaxClicks, activeInt, link.IOSURL, link.AndroidURL,
		string(localeJSON), string(variantsJSON), link.UpdatedAt, link.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update link: %w", err)
	}

	s.cache.Set(link.Domain, link.Slug, link)
	return link, nil
}

func (s *LinkService) Delete(id, username string, isAdmin bool) error {
	link, err := s.GetByID(id)
	if err != nil {
		return err
	}

	if !isAdmin && link.CreatedBy != username {
		return fmt.Errorf("unauthorized to delete this link")
	}

	query := `DELETE FROM links WHERE id = ?`
	if _, err := s.db.Exec(query, id); err != nil {
		return fmt.Errorf("failed to delete link: %w", err)
	}

	s.cache.Delete(link.Domain, link.Slug)

	if s.webhooks != nil {
		s.webhooks.Dispatch("link.deleted", link, link.CreatedBy)
	}
	return nil
}

func (s *LinkService) ValidateCustomSlug(slug string) error {
	if len(slug) < 2 || len(slug) > 64 {
		return fmt.Errorf("custom slug must be between 2 and 64 characters")
	}
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("custom slug can only contain letters, numbers, hyphens, and underscores")
	}
	if reservedSlugs[strings.ToLower(slug)] {
		return fmt.Errorf("slug '%s' is a reserved system path", slug)
	}
	return nil
}

func (s *LinkService) generateUniqueSlug(domain string) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		slug := generateRandomSlug(6)
		if reservedSlugs[strings.ToLower(slug)] {
			continue
		}
		existing, err := s.GetBySlugRaw(domain, slug)
		if err != nil || existing == nil {
			return slug, nil
		}
	}
	return generateRandomSlug(8), nil
}

func generateRandomSlug(length int) string {
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			b[i] = charset[i%len(charset)]
		} else {
			b[i] = charset[num.Int64()]
		}
	}
	return string(b)
}

func scanLink(row *sql.Row) (*models.Link, error) {
	var l models.Link
	var pinHash, tagsStr, localeStr, variantsStr string
	var isActive int
	err := row.Scan(
		&l.ID, &l.Slug, &l.Domain, &l.TargetURL, &l.OriginalURL, &l.Title, &l.FolderID,
		&tagsStr, &pinHash, &l.RedirectType, &l.ExpiresAt, &l.MaxClicks, &l.ClickCount,
		&l.LastClickedAt, &l.CreatedBy, &isActive, &l.IOSURL, &l.AndroidURL,
		&localeStr, &variantsStr, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("link not found")
		}
		return nil, err
	}
	l.PinHash = pinHash
	l.HasPIN = pinHash != ""
	l.IsActive = isActive == 1
	_ = json.Unmarshal([]byte(tagsStr), &l.Tags)
	_ = json.Unmarshal([]byte(localeStr), &l.LocaleRouting)
	_ = json.Unmarshal([]byte(variantsStr), &l.ABVariants)
	return &l, nil
}

func scanLinks(rows *sql.Rows) ([]models.Link, error) {
	var links []models.Link
	for rows.Next() {
		var l models.Link
		var pinHash, tagsStr, localeStr, variantsStr string
		var isActive int
		if err := rows.Scan(
			&l.ID, &l.Slug, &l.Domain, &l.TargetURL, &l.OriginalURL, &l.Title, &l.FolderID,
			&tagsStr, &pinHash, &l.RedirectType, &l.ExpiresAt, &l.MaxClicks, &l.ClickCount,
			&l.LastClickedAt, &l.CreatedBy, &isActive, &l.IOSURL, &l.AndroidURL,
			&localeStr, &variantsStr, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, err
		}
		l.PinHash = pinHash
		l.HasPIN = pinHash != ""
		l.IsActive = isActive == 1
		_ = json.Unmarshal([]byte(tagsStr), &l.Tags)
		_ = json.Unmarshal([]byte(localeStr), &l.LocaleRouting)
		_ = json.Unmarshal([]byte(variantsStr), &l.ABVariants)
		links = append(links, l)
	}
	return links, nil
}

func (s *LinkService) checkDomainBlocked(targetURL string) error {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return err
	}
	domain := strings.ToLower(parsed.Hostname())

	var count int
	query := `SELECT COUNT(*) FROM blocked_domains WHERE domain = ?`
	_ = s.db.QueryRow(query, domain).Scan(&count)
	if count > 0 {
		return fmt.Errorf("destination domain '%s' has been quarantined by policy", domain)
	}
	return nil
}
