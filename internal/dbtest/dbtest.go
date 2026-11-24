// Package dbtest gives each test a fresh, fully migrated database.
//
// Migrating per test cost minutes in CI (internal/routes 257 s, run 1275), so
// the migrations run once per schema into a template database named after
// their hash, and each test gets a copy: CREATE DATABASE ... TEMPLATE.
//
// Set TEST_DATABASE_URL to a server the tests may create and drop databases
// on, e.g. postgres://postgres:postgres@localhost:5432/postgres. Without it the
// DB tests skip -- unless REQUIRE_DB_TESTS=1, which CI sets so a misconfigured
// job fails loudly instead of passing by skipping everything.
package dbtest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/migrations"
)

// templateLock is the advisory lock that serializes building the template
// across test packages, which `go test ./...` runs as concurrent processes.
const templateLock = 0x726967656c // "rigel"

var (
	templateOnce sync.Once
	templateName string
	templateErr  error
)

func New(t testing.TB) *db.Store {
	t.Helper()
	store, _ := NewWithDSN(t)
	return store
}

// NewWithDSN is New, also returning the database's URL (for migration tests).
func NewWithDSN(t testing.TB) (*db.Store, string) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		if os.Getenv("REQUIRE_DB_TESTS") == "1" {
			t.Fatal("REQUIRE_DB_TESTS=1 but TEST_DATABASE_URL is not set")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	defer admin.Close(ctx)

	templateOnce.Do(func() { templateName, templateErr = ensureTemplate(base) })
	if templateErr != nil {
		t.Fatalf("migrated template database: %v", templateErr)
	}

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "rigel_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+templateName); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	dsn, err := withDatabase(base, name)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	store, err := db.Open(ctx, dsn, 4)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, base)
		if err != nil {
			t.Logf("drop %s: %v", name, err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop %s: %v", name, err)
		}
	})
	return store, dsn
}

// ensureTemplate returns the name of a database holding every migration,
// building it when this schema has none yet. The name carries a hash of the
// embedded migrations, so a changed or added migration builds a new one and
// the stale ones are dropped. It is built under another name and renamed when
// complete, so a failed migration never leaves a half-built template behind.
func ensureTemplate(base string) (string, error) {
	sum, err := migrationsHash()
	if err != nil {
		return "", err
	}
	name := "rigel_tmpl_" + sum

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		return "", err
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "SELECT pg_advisory_lock($1)", templateLock); err != nil {
		return "", err
	}
	defer func() { _, _ = admin.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", templateLock) }()

	var exists bool
	if err := admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	rows, err := admin.Query(ctx, "SELECT datname FROM pg_database WHERE datname LIKE 'rigel\\_tmpl\\_%'")
	if err != nil {
		return "", err
	}
	stale, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	for _, old := range stale {
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+old+" WITH (FORCE)"); err != nil {
			return "", fmt.Errorf("drop stale template %s: %w", old, err)
		}
	}

	build := name + "_build"
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+build); err != nil {
		return "", err
	}
	dsn, err := withDatabase(base, build)
	if err != nil {
		return "", err
	}
	if err := db.Migrate(dsn); err != nil {
		return "", fmt.Errorf("migrate: %w", err)
	}
	// db.Migrate has closed its connections; a copy needs the template idle.
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+build+" RENAME TO "+name); err != nil {
		return "", err
	}
	return name, nil
}

// migrationsHash is the first 12 hex digits of a SHA-256 over every embedded
// migration's name and contents, in name order.
func migrationsHash() (string, error) {
	names, err := fs.Glob(migrations.MigrationsFiles, "*.sql")
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		b, err := fs.ReadFile(migrations.MigrationsFiles, n)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// withDatabase is base (TEST_DATABASE_URL) pointed at another database.
func withDatabase(base, name string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
