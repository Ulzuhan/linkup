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

// UpdateFolderRequest renames a folder or recolours it; an absent field is left alone.
type UpdateFolderRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}
