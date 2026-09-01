package models

type Folder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

type CreateFolderRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}
