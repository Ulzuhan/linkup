package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var errNotReady = errors.New("SQLite readiness unavailable")

// Ready checks the application's pool and the current database file, without
// reading user rows, creating a database, migrating, or testing write access.
// One fresh read-only connection avoids masking deleted/replaced files behind
// cached handles. The time budget includes contention for the single probe.
func (db *DB) Ready(parent context.Context) error {
	if db == nil || db.DB == nil || db.probe == nil || db.closed.Load() {
		return errNotReady
	}
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	select {
	case db.probe <- struct{}{}:
		defer func() { <-db.probe }()
	case <-ctx.Done():
		return errNotReady
	}
	if ctx.Err() != nil || db.closed.Load() {
		return errNotReady
	}
	info, err := os.Lstat(db.path)
	if err != nil || !info.Mode().IsRegular() || db.identity == nil || !os.SameFile(info, db.identity) {
		return errNotReady
	}
	path, err := filepath.Abs(db.path)
	if err != nil {
		return errNotReady
	}
	// modernc honors SQLite URI parameters only with the file: prefix.
	uri := url.URL{Scheme: "file", Path: path}
	query := url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(100)", "query_only(1)"}}
	uri.RawQuery = query.Encode()
	reader, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return errNotReady
	}
	defer reader.Close()
	reader.SetMaxOpenConns(1)
	reader.SetMaxIdleConns(0)
	conn, err := reader.Conn(ctx)
	if err != nil {
		return errNotReady
	}
	defer conn.Close()
	var version int
	if conn.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version) != nil {
		return errNotReady
	}
	for _, statement := range readinessColumns {
		// This driver does not cancel PrepareContext itself. Bound lock waits
		// via busy_timeout and check cancellation around each preparation.
		if ctx.Err() != nil {
			return errNotReady
		}
		prepared, err := conn.PrepareContext(ctx, statement)
		if err != nil {
			return errNotReady
		}
		prepared.Close()
		if ctx.Err() != nil {
			return errNotReady
		}
	}
	// A concurrent application Close or path replacement cannot leave a green
	// response based solely on the independent reader.
	current, err := os.Lstat(db.path)
	if err != nil || !os.SameFile(current, db.identity) || db.closed.Load() || ctx.Err() != nil {
		return errNotReady
	}
	return nil
}

var readinessColumns = []string{
	`SELECT id,subject,sid,username,access_token,expires_at FROM oidc_sessions WHERE 0`,
	`SELECT jti,expires_at FROM oidc_logout_jtis WHERE 0`,
	`SELECT id,slug,domain,target_url,original_url,title,folder_id,tags,pin_hash,redirect_type,expires_at,max_clicks,click_count,last_clicked_at,created_by,is_active,ios_url,android_url,locale_routing,ab_variants,created_at,updated_at FROM links WHERE 0`,
	`SELECT id,domain,reason,created_at FROM blocked_domains WHERE 0`,
	`SELECT id,domain,created_by,is_verified,created_at FROM custom_domains WHERE 0`,
	`SELECT id,name,color,created_by,created_at FROM folders WHERE 0`,
	`SELECT id,user_id,name,key_prefix,key_hash,last_used_at,created_at FROM api_keys WHERE 0`,
	`SELECT id,user_id,url,secret,events,is_active,created_at FROM webhooks WHERE 0`,
}
