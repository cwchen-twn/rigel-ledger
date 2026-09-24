package db_test

import (
	"context"
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/dbtest"
)

// Every migration after 000001 must go down and up again over real data: the first migration
// written after the first deployment, applied to a database with users.
func TestAccountsAdminMigrationRoundTrip(t *testing.T) {
	store, dsn := dbtest.NewWithDSN(t)
	ctx := context.Background()
	if _, err := store.Pool.Exec(ctx, `INSERT INTO users (username, email, password_hash) VALUES ('alice', 'Alice@example.com', 'x')`); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive, and '' may repeat.
	if _, err := store.Pool.Exec(ctx, `INSERT INTO users (username, email) VALUES ('alice2', 'alice@EXAMPLE.com')`); err == nil {
		t.Fatal("the same address in another case was accepted")
	}
	if _, err := store.Pool.Exec(ctx, `INSERT INTO users (username) VALUES ('p1'), ('p2')`); err != nil {
		t.Fatalf("two users without an address: %v", err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE users SET initialized_at = now() WHERE username = 'p1'`); err == nil {
		t.Fatal("initialized without an address")
	}

	store.Close()
	if err := db.MigrateTo(dsn, 1); err != nil { // every down file back to 000001
		t.Fatalf("down: %v", err)
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("up again: %v", err)
	}
	store, err := db.Open(ctx, dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var n int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("users after the round trip = %d (%v); the ones without an address are dropped", n, err)
	}
	if _, err := store.GetSystemSettings(ctx); err != nil {
		t.Fatalf("settings row after the round trip: %v", err)
	}
}
