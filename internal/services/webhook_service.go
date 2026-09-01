package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kaicorplabs/linkup/internal/database"
	"github.com/kaicorplabs/linkup/internal/models"
)

type WebhookService struct {
	db         *database.DB
	httpClient *http.Client
}

func NewWebhookService(db *database.DB) *WebhookService {
	return &WebhookService{
		db: db,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Create registers a new webhook endpoint
func (s *WebhookService) Create(req models.CreateWebhookRequest, username string) (*models.Webhook, error) {
	url := strings.TrimSpace(req.URL)
	if url == "" || (!strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://")) {
		return nil, fmt.Errorf("valid webhook HTTP/HTTPS URL is required")
	}

	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		b := make([]byte, 24)
		_, _ = rand.Read(b)
		secret = hex.EncodeToString(b)
	}

	events := strings.Join(req.Events, ",")
	if events == "" {
		events = "link.created,link.expired,link.self_destructed"
	}

	wh := &models.Webhook{
		ID:        uuid.New().String(),
		UserID:    username,
		URL:       url,
		Secret:    secret,
		Events:    events,
		IsActive:  true,
		CreatedAt: time.Now().Unix(),
	}

	query := `INSERT INTO webhooks (id, user_id, url, secret, events, is_active, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, wh.ID, wh.UserID, wh.URL, wh.Secret, wh.Events, 1, wh.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save webhook: %w", err)
	}

	return wh, nil
}

// List returns webhooks for a user
func (s *WebhookService) List(username string, isAdmin bool) ([]models.Webhook, error) {
	var query string
	var args []interface{}

	if isAdmin {
		query = `SELECT id, user_id, url, secret, events, is_active, created_at FROM webhooks ORDER BY created_at DESC`
	} else {
		query = `SELECT id, user_id, url, secret, events, is_active, created_at FROM webhooks WHERE user_id = ? ORDER BY created_at DESC`
		args = append(args, username)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []models.Webhook
	for rows.Next() {
		var w models.Webhook
		var isActive int
		if err := rows.Scan(&w.ID, &w.UserID, &w.URL, &w.Secret, &w.Events, &isActive, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.IsActive = isActive == 1
		webhooks = append(webhooks, w)
	}
	return webhooks, nil
}

// Delete removes a webhook
func (s *WebhookService) Delete(id, username string, isAdmin bool) error {
	var query string
	var args []interface{}

	if isAdmin {
		query = `DELETE FROM webhooks WHERE id = ?`
		args = append(args, id)
	} else {
		query = `DELETE FROM webhooks WHERE id = ? AND user_id = ?`
		args = append(args, id, username)
	}

	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("webhook not found or unauthorized")
	}
	return nil
}

// Dispatch sends a webhook payload asynchronously to all subscribed endpoints for this user
func (s *WebhookService) Dispatch(event string, data interface{}, userID string) {
	go func() {
		query := `SELECT id, url, secret, events FROM webhooks WHERE is_active = 1 AND (user_id = ? OR user_id = 'global')`
		rows, err := s.db.Query(query, userID)
		if err != nil {
			return
		}
		defer rows.Close()

		payload := models.WebhookPayload{
			Event:     event,
			Timestamp: time.Now().Unix(),
			Data:      data,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return
		}

		for rows.Next() {
			var id, targetURL, secret, events string
			if err := rows.Scan(&id, &targetURL, &secret, &events); err != nil {
				continue
			}

			if !isSubscribedToEvent(events, event) {
				continue
			}

			// Send in parallel with retry
			go s.deliverWebhook(targetURL, secret, event, payloadBytes)
		}
	}()
}

func (s *WebhookService) deliverWebhook(targetURL, secret, event string, body []byte) {
	signature := computeHMACSHA256(body, secret)

	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(body))
		if err != nil {
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "LinkUp-Webhook/2.0")
		req.Header.Set("X-LinkUp-Event", event)
		req.Header.Set("X-LinkUp-Signature", "sha256="+signature)

		resp, err := s.httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return // Success!
			}
		}

		// Exponential backoff retry
		time.Sleep(time.Duration(attempt*attempt) * 500 * time.Millisecond)
	}

	log.Printf("[WEBHOOK] Delivery failed after 3 attempts to %s for event %s", targetURL, event)
}

func isSubscribedToEvent(subscribedEvents, event string) bool {
	if subscribedEvents == "" || subscribedEvents == "*" {
		return true
	}
	for _, e := range strings.Split(subscribedEvents, ",") {
		if strings.TrimSpace(e) == event || strings.TrimSpace(e) == "*" {
			return true
		}
	}
	return false
}

func computeHMACSHA256(message []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}
