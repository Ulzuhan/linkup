package services

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/google/uuid"
)

type DomainService struct {
	db *database.DB
}

func NewDomainService(db *database.DB) *DomainService {
	return &DomainService{db: db}
}

func (s *DomainService) Create(rawDomain, username string) (*models.CustomDomain, error) {
	domain := strings.ToLower(strings.TrimSpace(rawDomain))
	if domain == "" {
		return nil, fmt.Errorf("domain cannot be empty")
	}

	// Remove protocol if user included it
	if strings.Contains(domain, "://") {
		u, err := url.Parse(domain)
		if err == nil && u.Hostname() != "" {
			domain = u.Hostname()
		}
	}
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.Split(domain, "/")[0]
	domain = strings.Split(domain, ":")[0]

	if len(domain) < 3 || !strings.Contains(domain, ".") {
		return nil, fmt.Errorf("invalid domain format (e.g. go.example.com)")
	}

	cd := &models.CustomDomain{
		ID:         uuid.New().String(),
		Domain:     domain,
		CreatedBy:  username,
		IsVerified: true, // auto-verified in sovereign self-hosted setups
		CreatedAt:  time.Now().Unix(),
	}

	query := `INSERT INTO custom_domains (id, domain, created_by, is_verified, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, cd.ID, cd.Domain, cd.CreatedBy, 1, cd.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("domain is already registered or invalid: %w", err)
	}

	return cd, nil
}

func (s *DomainService) List(username string, isAdmin bool) ([]models.CustomDomain, error) {
	var query string
	var args []interface{}

	if isAdmin {
		query = `SELECT id, domain, created_by, is_verified, created_at FROM custom_domains ORDER BY created_at DESC`
	} else {
		query = `SELECT id, domain, created_by, is_verified, created_at FROM custom_domains WHERE created_by = ? ORDER BY created_at DESC`
		args = append(args, username)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []models.CustomDomain
	for rows.Next() {
		var d models.CustomDomain
		var isVerified int
		if err := rows.Scan(&d.ID, &d.Domain, &d.CreatedBy, &isVerified, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.IsVerified = isVerified == 1
		domains = append(domains, d)
	}
	return domains, nil
}

func (s *DomainService) Delete(id, username string, isAdmin bool) error {
	var query string
	var args []interface{}

	if isAdmin {
		query = `DELETE FROM custom_domains WHERE id = ?`
		args = append(args, id)
	} else {
		query = `DELETE FROM custom_domains WHERE id = ? AND created_by = ?`
		args = append(args, id, username)
	}

	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("domain not found or unauthorized")
	}
	return nil
}
