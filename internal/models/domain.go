package models

type CustomDomain struct {
	ID         string `json:"id"`
	Domain     string `json:"domain"`
	CreatedBy  string `json:"created_by"`
	IsVerified bool   `json:"is_verified"`
	CreatedAt  int64  `json:"created_at"`
}

type CreateDomainRequest struct {
	Domain string `json:"domain"`
}
