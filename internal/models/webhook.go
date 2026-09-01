package models

type Webhook struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	URL       string `json:"url"`
	Secret    string `json:"-"`
	Events    string `json:"events"` // Comma-separated: "link.created,link.expired,link.self_destructed"
	IsActive  bool   `json:"is_active"`
	CreatedAt int64  `json:"created_at"`
}

type CreateWebhookRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events"`
}

type WebhookPayload struct {
	Event     string      `json:"event"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data"`
}
