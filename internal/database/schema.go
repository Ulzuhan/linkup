package database

import "fmt"

func (db *DB) Migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS links (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL,
			domain TEXT NOT NULL DEFAULT '',
			target_url TEXT NOT NULL,
			original_url TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			folder_id TEXT,
			tags TEXT NOT NULL DEFAULT '[]',
			pin_hash TEXT NOT NULL DEFAULT '',
			redirect_type INTEGER NOT NULL DEFAULT 302,
			expires_at INTEGER,
			max_clicks INTEGER,
			click_count INTEGER NOT NULL DEFAULT 0,
			last_clicked_at INTEGER,
			created_by TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			ios_url TEXT NOT NULL DEFAULT '',
			android_url TEXT NOT NULL DEFAULT '',
			locale_routing TEXT NOT NULL DEFAULT '{}',
			ab_variants TEXT NOT NULL DEFAULT '[]',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_links_domain_slug ON links(domain, slug);`,
		`CREATE INDEX IF NOT EXISTS idx_links_created_by ON links(created_by);`,
		`CREATE INDEX IF NOT EXISTS idx_links_folder_id ON links(folder_id);`,
		`CREATE INDEX IF NOT EXISTS idx_links_expires_at ON links(expires_at);`,
		
		`CREATE TABLE IF NOT EXISTS blocked_domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT UNIQUE NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_blocked_domains_domain ON blocked_domains(domain);`,

		`CREATE TABLE IF NOT EXISTS custom_domains (
			id TEXT PRIMARY KEY,
			domain TEXT UNIQUE NOT NULL,
			created_by TEXT NOT NULL,
			is_verified INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_custom_domains_user ON custom_domains(created_by);`,

		`CREATE TABLE IF NOT EXISTS folders (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			color TEXT NOT NULL DEFAULT '#06b6d4',
			created_by TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_folders_user ON folders(created_by);`,

		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			key_prefix TEXT NOT NULL,
			key_hash TEXT NOT NULL,
			last_used_at INTEGER,
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);`,

		`CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			url TEXT NOT NULL,
			secret TEXT NOT NULL DEFAULT '',
			events TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_webhooks_user ON webhooks(user_id);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("migration query failed (%s): %w", query, err)
		}
	}

	return nil
}
