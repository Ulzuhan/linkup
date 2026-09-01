package models

type APIKey struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	KeyPrefix  string `json:"key_prefix"`
	KeyHash    string `json:"-"` // Never serialized
	LastUsedAt *int64 `json:"last_used_at,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

type APIKeyCreatedResponse struct {
	APIKey APIKey `json:"api_key"`
	Secret string `json:"secret"` // Displayed only once upon creation
}
