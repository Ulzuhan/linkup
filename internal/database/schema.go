package database

import "fmt"

func (db *DB) Migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS links (
			id TEXT PRIMARY KEY,
			slug TEXT UNIQUE NOT NULL,
			target_url TEXT NOT NULL,
			original_url TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			pin_hash TEXT NOT NULL DEFAULT '',
			redirect_type INTEGER NOT NULL DEFAULT 302,
			expires_at INTEGER,
			max_clicks INTEGER,
			click_count INTEGER NOT NULL DEFAULT 0,
			last_clicked_at INTEGER,
			created_by TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_links_slug ON links(slug);`,
		`CREATE INDEX IF NOT EXISTS idx_links_created_by ON links(created_by);`,
		`CREATE INDEX IF NOT EXISTS idx_links_expires_at ON links(expires_at);`,
		`CREATE TABLE IF NOT EXISTS blocked_domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT UNIQUE NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_blocked_domains_domain ON blocked_domains(domain);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("migration query failed (%s): %w", query, err)
		}
	}

	return nil
}
