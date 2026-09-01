package services

import (
	"crypto/rand"
	"database/sql"
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
		"health":      true,
		"healthz":     true,
		"favicon.ico": true,
		"robots.txt":  true,
		"login":       true,
		"logout":      true,
		"settings":    true,
		"admin":       true,
	}
	charset = "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ" // base56 (unambiguous characters)
)

type LinkService struct {
	db       *database.DB
	cache    *LinkCache
	ownHost  string
}

func NewLinkService(db *database.DB, cache *LinkCache, ownHost string) *LinkService {
	return &LinkService{
		db:      db,
		cache:   cache,
		ownHost: ownHost,
	}
}

// Create creates a new shortened link with cleaned target URL.
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

	// 3. Determine slug
	slug := strings.TrimSpace(req.CustomSlug)
	if slug != "" {
		if err := s.ValidateCustomSlug(slug); err != nil {
			return nil, nil, err
		}
		// Check uniqueness
		existing, err := s.GetBySlugRaw(slug)
		if err == nil && existing != nil {
			return nil, nil, fmt.Errorf("custom slug '%s' is already in use", slug)
		}
	} else {
		// Generate random unique slug
		generatedSlug, err := s.generateUniqueSlug()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate unique slug: %w", err)
		}
		slug = generatedSlug
	}

	// 4. Handle PIN
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

	// 5. Expiration & Click Budget
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

	now := time.Now().Unix()
	link := &models.Link{
		ID:           uuid.New().String(),
		Slug:         slug,
		TargetURL:    cleanTarget,
		OriginalURL:  req.URL,
		Title:        title,
		HasPIN:       hasPIN,
		PinHash:      pinHash,
		RedirectType: redirectType,
		ExpiresAt:    expiresAt,
		MaxClicks:    req.MaxClicks,
		ClickCount:   0,
		CreatedBy:    createdBy,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// 6. Insert into database
	query := `INSERT INTO links (
		id, slug, target_url, original_url, title, pin_hash, redirect_type, 
		expires_at, max_clicks, click_count, created_by, is_active, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.Exec(query,
		link.ID, link.Slug, link.TargetURL, link.OriginalURL, link.Title, link.PinHash,
		link.RedirectType, link.ExpiresAt, link.MaxClicks, link.ClickCount,
		link.CreatedBy, 1, link.CreatedAt, link.UpdatedAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to save link to database: %w", err)
	}

	// 7. Store in cache
	s.cache.Set(link.Slug, link)

	return link, strippedParams, nil
}

// Resolve retrieves and validates a link for public redirection.
func (s *LinkService) Resolve(slug string) (*models.Link, error) {
	// Try cache first
	if cached, ok := s.cache.Get(slug); ok {
		if cached.IsExpired() {
			s.cache.Delete(slug)
			return cached, fmt.Errorf("link has expired or reached click limit")
		}
		return cached, nil
	}

	// Database query
	link, err := s.GetBySlugRaw(slug)
	if err != nil {
		return nil, err
	}

	if link.IsExpired() {
		return link, fmt.Errorf("link has expired or reached click limit")
	}

	// Update cache
	s.cache.Set(slug, link)
	return link, nil
}

// RecordClick increments the link click counter asynchronously without blocking.
func (s *LinkService) RecordClick(linkID, slug string) {
	go func() {
		now := time.Now().Unix()
		query := `UPDATE links SET click_count = click_count + 1, last_clicked_at = ? WHERE id = ?`
		_, _ = s.db.Exec(query, now, linkID)

		// Check if we hit max clicks limit to invalidate cache
		updated, err := s.GetByID(linkID)
		if err == nil && updated != nil {
			if updated.IsExpired() {
				s.cache.Delete(slug)
			} else {
				s.cache.Set(slug, updated)
			}
		}
	}()
}

// GetBySlugRaw retrieves link struct from DB by slug
func (s *LinkService) GetBySlugRaw(slug string) (*models.Link, error) {
	query := `SELECT id, slug, target_url, original_url, title, pin_hash, redirect_type,
		expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, created_at, updated_at
		FROM links WHERE slug = ?`

	var l models.Link
	var pinHash string
	var isActive int
	err := s.db.QueryRow(query, slug).Scan(
		&l.ID, &l.Slug, &l.TargetURL, &l.OriginalURL, &l.Title, &pinHash, &l.RedirectType,
		&l.ExpiresAt, &l.MaxClicks, &l.ClickCount, &l.LastClickedAt, &l.CreatedBy, &isActive, &l.CreatedAt, &l.UpdatedAt,
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
	return &l, nil
}

// GetByID retrieves link by ID
func (s *LinkService) GetByID(id string) (*models.Link, error) {
	query := `SELECT id, slug, target_url, original_url, title, pin_hash, redirect_type,
		expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, created_at, updated_at
		FROM links WHERE id = ?`

	var l models.Link
	var pinHash string
	var isActive int
	err := s.db.QueryRow(query, id).Scan(
		&l.ID, &l.Slug, &l.TargetURL, &l.OriginalURL, &l.Title, &pinHash, &l.RedirectType,
		&l.ExpiresAt, &l.MaxClicks, &l.ClickCount, &l.LastClickedAt, &l.CreatedBy, &isActive, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	l.PinHash = pinHash
	l.HasPIN = pinHash != ""
	l.IsActive = isActive == 1
	return &l, nil
}

// ListByUser retrieves links created by a specific user or all links if admin
func (s *LinkService) ListByUser(username string, isAdmin bool) ([]models.Link, error) {
	var query string
	var rows *sql.Rows
	var err error

	if isAdmin {
		query = `SELECT id, slug, target_url, original_url, title, pin_hash, redirect_type,
			expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, created_at, updated_at
			FROM links ORDER BY created_at DESC`
		rows, err = s.db.Query(query)
	} else {
		query = `SELECT id, slug, target_url, original_url, title, pin_hash, redirect_type,
			expires_at, max_clicks, click_count, last_clicked_at, created_by, is_active, created_at, updated_at
			FROM links WHERE created_by = ? ORDER BY created_at DESC`
		rows, err = s.db.Query(query, username)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []models.Link
	for rows.Next() {
		var l models.Link
		var pinHash string
		var isActive int
		if err := rows.Scan(
			&l.ID, &l.Slug, &l.TargetURL, &l.OriginalURL, &l.Title, &pinHash, &l.RedirectType,
			&l.ExpiresAt, &l.MaxClicks, &l.ClickCount, &l.LastClickedAt, &l.CreatedBy, &isActive, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, err
		}
		l.PinHash = pinHash
		l.HasPIN = pinHash != ""
		l.IsActive = isActive == 1
		links = append(links, l)
	}

	return links, nil
}

// Update updates a link's configuration
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

	link.UpdatedAt = time.Now().Unix()

	query := `UPDATE links SET target_url = ?, title = ?, pin_hash = ?, expires_at = ?, max_clicks = ?, is_active = ?, updated_at = ? WHERE id = ?`
	activeInt := 0
	if link.IsActive {
		activeInt = 1
	}

	_, err = s.db.Exec(query, link.TargetURL, link.Title, link.PinHash, link.ExpiresAt, link.MaxClicks, activeInt, link.UpdatedAt, link.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update link: %w", err)
	}

	// Update cache
	s.cache.Set(link.Slug, link)

	return link, nil
}

// Delete removes a link
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

	// Invalidate cache
	s.cache.Delete(link.Slug)
	return nil
}

// ValidateCustomSlug validates format and reserved words
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

func (s *LinkService) generateUniqueSlug() (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		slug := generateRandomSlug(6)
		if reservedSlugs[strings.ToLower(slug)] {
			continue
		}
		existing, err := s.GetBySlugRaw(slug)
		if err != nil || existing == nil {
			return slug, nil
		}
	}
	// Fallback to 8-chars if 6-chars collision happened
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
