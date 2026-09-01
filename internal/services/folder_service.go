package services

import (
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

func (s *FolderService) Delete(id, username string, isAdmin bool) error {
	var query string
	var args []interface{}

	if isAdmin {
		query = `DELETE FROM folders WHERE id = ?`
		args = append(args, id)
	} else {
		query = `DELETE FROM folders WHERE id = ? AND created_by = ?`
		args = append(args, id, username)
	}

	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("folder not found or unauthorized")
	}

	// Unlink links in this folder
	_, _ = s.db.Exec(`UPDATE links SET folder_id = NULL WHERE folder_id = ?`, id)

	return nil
}
