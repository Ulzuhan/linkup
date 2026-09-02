package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/models"
	"github.com/google/uuid"
)

type FolderService struct {
	db *database.DB
}

func NewFolderService(db *database.DB) *FolderService {
	return &FolderService{db: db}
}

func (s *FolderService) Create(name, color, username string) (*models.Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}

	color = strings.TrimSpace(color)
	if color == "" {
		color = "#06b6d4" // KaiCorp Cyan default
	}

	f := &models.Folder{
		ID:        uuid.New().String(),
		Name:      name,
		Color:     color,
		CreatedBy: username,
		CreatedAt: time.Now().Unix(),
	}

	query := `INSERT INTO folders (id, name, color, created_by, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, f.ID, f.Name, f.Color, f.CreatedBy, f.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return f, nil
}

func (s *FolderService) List(username string, isAdmin bool) ([]models.Folder, error) {
	var query string
	var args []interface{}

	if isAdmin {
		query = `SELECT id, name, color, created_by, created_at FROM folders ORDER BY name ASC`
	} else {
		query = `SELECT id, name, color, created_by, created_at FROM folders WHERE created_by = ? ORDER BY name ASC`
		args = append(args, username)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []models.Folder
	for rows.Next() {
		var f models.Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.Color, &f.CreatedBy, &f.CreatedAt); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, nil
}

// Get returns one folder, or nil when it does not exist.
func (s *FolderService) Get(id string) (*models.Folder, error) {
	var f models.Folder
	err := s.db.QueryRow(`SELECT id, name, color, created_by, created_at FROM folders WHERE id = ?`, id).
		Scan(&f.ID, &f.Name, &f.Color, &f.CreatedBy, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// Update renames or recolours a folder. Only its owner can, unless admin.
func (s *FolderService) Update(id string, req models.UpdateFolderRequest, username string, isAdmin bool) (*models.Folder, error) {
	f, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if f == nil || (!isAdmin && f.CreatedBy != username) {
		return nil, fmt.Errorf("folder not found or unauthorized")
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("folder name cannot be empty")
		}
		f.Name = name
	}
	if req.Color != nil {
		if c := strings.TrimSpace(*req.Color); c != "" {
			f.Color = c
		}
	}
	if _, err := s.db.Exec(`UPDATE folders SET name = ?, color = ? WHERE id = ?`, f.Name, f.Color, f.ID); err != nil {
		return nil, fmt.Errorf("failed to update folder: %w", err)
	}
	return f, nil
}

// Delete removes a folder and sends its links back to "no folder". The two
// statements are one transaction: a folder that disappears while its links
// still point at it would leave them filtered out of every view, and the
// old code did exactly that whenever the second statement failed.
func (s *FolderService) Delete(id, username string, isAdmin bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var res sql.Result
	if isAdmin {
		res, err = tx.Exec(`DELETE FROM folders WHERE id = ?`, id)
	} else {
		res, err = tx.Exec(`DELETE FROM folders WHERE id = ? AND created_by = ?`, id, username)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("folder not found or unauthorized")
	}
	if _, err := tx.Exec(`UPDATE links SET folder_id = NULL WHERE folder_id = ?`, id); err != nil {
		return fmt.Errorf("failed to release the folder's links: %w", err)
	}
	return tx.Commit()
}
