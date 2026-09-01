package services

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kaicorplabs/linkup/internal/database"
	"github.com/kaicorplabs/linkup/internal/models"
)

type APIKeyService struct {
	db      *database.DB
	isAdmin func(string) bool
}

func NewAPIKeyService(db *database.DB, isAdmin func(string) bool) *APIKeyService {
	return &APIKeyService{db: db, isAdmin: isAdmin}
}

// Create generates a new secret API key and stores its SHA-256 hash
func (s *APIKeyService) Create(name, username string) (*models.APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Default API Key"
	}

	randomBytes := make([]byte, 24)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	rawSecret := "lk_live_" + hex.EncodeToString(randomBytes)
	keyPrefix := rawSecret[:16] + "..."
	keyHash := hashAPIKey(rawSecret)

	apiKey := &models.APIKey{
		ID:        uuid.New().String(),
		UserID:    username,
		Name:      name,
		KeyPrefix: keyPrefix,
		KeyHash:   keyHash,
		CreatedAt: time.Now().Unix(),
	}

	query := `INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, apiKey.ID, apiKey.UserID, apiKey.Name, apiKey.KeyPrefix, apiKey.KeyHash, apiKey.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("failed to save API key: %w", err)
	}

	return apiKey, rawSecret, nil
}

// List returns all API keys for user
func (s *APIKeyService) List(username string, isAdmin bool) ([]models.APIKey, error) {
	var query string
	var args []interface{}

	if isAdmin {
		query = `SELECT id, user_id, name, key_prefix, last_used_at, created_at FROM api_keys ORDER BY created_at DESC`
	} else {
		query = `SELECT id, user_id, name, key_prefix, last_used_at, created_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`
		args = append(args, username)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.LastUsedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// Delete revokes an API key
func (s *APIKeyService) Delete(id, username string, isAdmin bool) error {
	var query string
	var args []interface{}

	if isAdmin {
		query = `DELETE FROM api_keys WHERE id = ?`
		args = append(args, id)
	} else {
		query = `DELETE FROM api_keys WHERE id = ? AND user_id = ?`
		args = append(args, id, username)
	}

	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("API key not found or unauthorized")
	}
	return nil
}

// ValidateKey checks a Bearer token against stored hashes and returns user session
func (s *APIKeyService) ValidateKey(token string) (*models.UserSession, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "lk_live_") {
		return nil, errors.New("invalid API key format")
	}

	keyHash := hashAPIKey(token)
	query := `SELECT id, user_id FROM api_keys WHERE key_hash = ?`

	var keyID, userID string
	err := s.db.QueryRow(query, keyHash).Scan(&keyID, &userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("invalid or revoked API key")
		}
		return nil, err
	}

	// Update last used timestamp in background
	go func() {
		now := time.Now().Unix()
		_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, now, keyID)
	}()

	isAdmin := s.isAdmin(userID)

	return &models.UserSession{
		UserID:    userID,
		Username:  userID,
		Email:     userID,
		IsAdmin:   isAdmin,
		CreatedAt: time.Now().Unix(),
	}, nil
}

func hashAPIKey(rawSecret string) string {
	hasher := sha256.New()
	hasher.Write([]byte(rawSecret))
	return hex.EncodeToString(hasher.Sum(nil))
}
