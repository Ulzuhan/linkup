package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
	path     string
	identity os.FileInfo
	probe    chan struct{}
	closed   atomic.Bool
}

func Open(dbPath string) (*DB, error) {
	dbPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
	}

	uri := url.URL{Scheme: "file", Path: dbPath}
	uri.RawQuery = url.Values{"_pragma": {"journal_mode(WAL)", "synchronous(NORMAL)", "busy_timeout(5000)", "foreign_keys(ON)"}}.Encode()
	sqlDB, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Calibrate connection pool for SQLite
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	db := &DB{DB: sqlDB, path: dbPath, probe: make(chan struct{}, 1)}
	if err := db.Migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}
	db.identity, err = os.Lstat(dbPath)
	if err != nil {
		sqlDB.Close()
		return nil, err
	}

	log.Printf("[DB] SQLite database initialized at %s (WAL mode enabled)", dbPath)
	return db, nil
}

// Close invalidates readiness before closing the writer's connection pool.
func (db *DB) Close() error { db.closed.Store(true); return db.DB.Close() }
