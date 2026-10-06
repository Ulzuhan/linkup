package tests

import (
	"context"
	"database/sql"
	"github.com/Ulzuhan/linkup/internal/config"
	"github.com/Ulzuhan/linkup/internal/database"
	"github.com/Ulzuhan/linkup/internal/handlers"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readinessDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private ?# SQLite.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func TestReadinessFreshReadOnlyAndFailures(t *testing.T) {
	for _, failure := range []string{"healthy", "closed", "missing", "partial", "column", "replacement", "corrupt", "unreadable"} {
		t.Run(failure, func(t *testing.T) {
			db, path := readinessDB(t)
			if _, err := db.Exec(`INSERT INTO folders VALUES('kept','private-data','blue','owner',1700000000)`); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "closed":
				db.Close()
			case "missing":
				db.SetMaxIdleConns(0) // Probes must not open the writer with CREATE.
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "partial":
				if _, err := db.Exec(`DROP TABLE webhooks`); err != nil {
					t.Fatal(err)
				}
			case "column":
				if _, err := db.Exec(`ALTER TABLE webhooks RENAME COLUMN secret TO other`); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("invalid SQLite header"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses file permissions")
				}
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(path, 0600) })
			}
			err := db.Ready(context.Background())
			if failure == "healthy" {
				if err != nil {
					t.Fatal(err)
				}
				var name, owner string
				var stamp int64
				if db.QueryRow(`SELECT name,created_by,created_at FROM folders WHERE id='kept'`).Scan(&name, &owner, &stamp) != nil || name != "private-data" || owner != "owner" || stamp != 1700000000 {
					t.Fatal("readiness changed user data")
				}
			} else if err == nil {
				t.Fatal("unavailable database accepted")
			}
			if failure == "missing" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("readiness created missing database")
				}
			}
		})
	}
}

func TestReadinessCancellationAndExclusiveLockBudget(t *testing.T) {
	db, path := readinessDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if db.Ready(ctx) == nil {
		t.Fatal("cancelled readiness accepted")
	}
	if _, err := db.Exec(`PRAGMA journal_mode=DELETE`); err != nil {
		t.Fatal(err)
	}
	uri := url.URL{Scheme: "file", Path: path}
	holder, err := sql.Open("sqlite", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	holder.SetMaxOpenConns(1)
	if _, err := holder.Exec(`BEGIN EXCLUSIVE`); err != nil {
		t.Fatal(err)
	}
	defer holder.Exec(`ROLLBACK`)
	start := time.Now()
	if db.Ready(context.Background()) == nil {
		t.Fatal("locked schema accepted")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("lock exceeded readiness budget")
	}
}

func TestReadinessHTTPDoesNotExposeDetailsAndLivenessIsIndependent(t *testing.T) {
	db, _ := readinessDB(t)
	router := handlers.NewRouter(&config.Config{}, db, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	healthy := get(t, router, "/healthz")
	if healthy.Code != 200 || healthy.Body.String() != `{"status":"healthy","service":"linkup","sqlite":"ready"}` || healthy.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("readiness marker absent")
	}
	db.Close()
	down := get(t, router, "/healthz")
	if down.Code != 503 || down.Body.String() != `{"status":"unavailable","service":"linkup"}` || strings.Contains(down.Body.String(), "private") {
		t.Fatal("unavailable response not generic")
	}
	if live := get(t, router, "/health"); live.Code != 200 {
		t.Fatal("liveness depends on SQLite")
	}
	// Existing valid printed slugs remain routed; no new top-level aliases.
}
