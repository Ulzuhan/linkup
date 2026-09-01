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
	"net/url"
	"strings"
	"time"

	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/google/uuid"
)

type WebhookService struct {
	db         *database.DB
	httpClient *http.Client
	// validateTarget gates every user-supplied URL. It is a field and not a
	// direct call so tests can reach an httptest server, which always binds
	// loopback and is therefore refused by the real validator. Production code
	// never replaces it; the only setter is named for what it is.
	validateTarget func(string) (*url.URL, error)
}

func NewWebhookService(db *database.DB) *WebhookService {
	return &WebhookService{
		db: db,
		// The hardened client: bounded and, above all, it does not follow
		// redirects. See egress.go for why that matters.
		httpClient:     NewOutboundClient(5 * time.Second),
		validateTarget: ValidateOutboundURL,
	}
}

// AllowReservedTargetsForTesting relaxes destination checks to scheme only.
//
// It exists so the delivery and signature tests can talk to an httptest server
// on loopback. Calling it from anything that is not a test re-opens the SSRF
// this validation closes, and the name is deliberately impossible to mistake
// for something reasonable in a code review.
func (s *WebhookService) AllowReservedTargetsForTesting() {
	s.validateTarget = func(raw string) (*url.URL, error) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidOutboundURL, err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("%w: scheme %q is not allowed", ErrInvalidOutboundURL, parsed.Scheme)
		}
		return parsed, nil
	}
}

// Create registers a new webhook endpoint
func (s *WebhookService) Create(req models.CreateWebhookRequest, username string) (*models.Webhook, error) {
	// Refuse anything we are not willing to call before it ever reaches the
	// database. A stored bad URL is a stored SSRF.
	parsed, err := s.validateTarget(req.URL)
	if err != nil {
		return nil, err
	}
	url := parsed.String()

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
	if _, err := s.db.Exec(query, wh.ID, wh.UserID, wh.URL, wh.Secret, wh.Events, 1, wh.CreatedAt); err != nil {
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
	// Validated AGAIN, on purpose. What was public when it was stored can point
	// somewhere private now; checking only at creation leaves DNS rebinding open.
	if _, err := s.validateTarget(targetURL); err != nil {
		log.Printf("[WEBHOOK] Refusing delivery for event %s: %v", event, err)
		return
	}

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
