// Package dbtest gives each test a fresh, fully migrated database.
//
// Set TEST_DATABASE_URL to a server the tests may create and drop databases
// on, e.g. postgres://postgres:postgres@localhost:5432/postgres. Without it the
// DB tests skip -- unless REQUIRE_DB_TESTS=1, which CI sets so a misconfigured
// job fails loudly instead of passing by skipping everything.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
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

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "rigel_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate test database: %v", err)
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
