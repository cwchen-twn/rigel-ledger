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

// 000008 folds the two runner kinds into one: the server runner's keys,
// connectors, tokens and connections go; a person's runner stays, as a
// plain runner, with its connections.
func TestOneRunnerMigration(t *testing.T) {
	store, dsn := dbtest.NewWithDSN(t)
	ctx := context.Background()
	store.Close()
	if err := db.MigrateTo(dsn, 7); err != nil {
		t.Fatalf("down to 000007: %v", err)
	}
	store, err := db.Open(ctx, dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var alice, bob, book int64
	_ = store.Pool.QueryRow(ctx, `INSERT INTO users (username, email, password_hash) VALUES ('alice', 'a@example.com', 'x') RETURNING id`).Scan(&alice)
	_ = store.Pool.QueryRow(ctx, `INSERT INTO users (username, email, password_hash) VALUES ('bob', 'b@example.com', 'x') RETURNING id`).Scan(&bob)
	_ = store.Pool.QueryRow(ctx, `INSERT INTO books (name, base_currency) VALUES ('B', 'TWD') RETURNING id`).Scan(&book)
	exec(`UPDATE users SET sync_mode = 'server' WHERE id = $1`, alice)
	exec(`INSERT INTO sessions (user_id, token_hash, kind, expires_at) VALUES ($1, 'h1', 'runner', now() + interval '1 day'),
	      ($2, 'h2', 'personal_runner', now() + interval '1 day')`, alice, bob)
	exec(`INSERT INTO runner_keys (public_key, owner_id) VALUES (decode(repeat('01', 32), 'hex'), NULL), (decode(repeat('02', 32), 'hex'), $1)`, bob)
	exec(`INSERT INTO runner_connectors (id, owner_id, name) VALUES ('fake', NULL, 'Fake'), ('fake', $1, 'Fake')`, bob)
	exec(`INSERT INTO connections (user_id, book_id, connector, sealed, key_id)
	      SELECT $1::BIGINT, $3::BIGINT, 'fake', '\x00'::BYTEA, id FROM runner_keys WHERE owner_id IS NULL
	      UNION ALL SELECT $2::BIGINT, $3::BIGINT, 'fake', '\x00'::BYTEA, id FROM runner_keys WHERE owner_id = $2::BIGINT`, alice, bob, book)
	store.Close()

	check := func() {
		t.Helper()
		store, err := db.Open(ctx, dsn, 2)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		var kinds, keys, conns, connectors int
		_ = store.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE kind = 'runner' AND user_id = $1),
		    (SELECT count(*) FROM runner_keys), (SELECT count(*) FROM connections WHERE user_id = $1),
		    (SELECT count(*) FROM runner_connectors) FROM sessions`, bob).Scan(&kinds, &keys, &conns, &connectors)
		if kinds != 1 || keys != 1 || conns != 1 || connectors != 1 {
			t.Fatalf("after 000008: bob's runner tokens %d, keys %d, bob's connections %d, connectors %d", kinds, keys, conns, connectors)
		}
		var total int
		_ = store.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind = 'runner'`).Scan(&total)
		if total != 1 {
			t.Fatalf("runner sessions = %d; the server runner's should be gone", total)
		}
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("up: %v", err)
	}
	check()
	if err := db.MigrateTo(dsn, 7); err != nil {
		t.Fatalf("down again: %v", err)
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("up again: %v", err)
	}
	check()
}

// 000016 adds the 'adjustment' source (#87); going down turns adjustments
// into manual transactions, even inside a closed period.
func TestAdjustmentSourceMigration(t *testing.T) {
	store, dsn := dbtest.NewWithDSN(t)
	ctx := context.Background()
	var book, cash, food, txn int64
	scan := func(dst *int64, sql string, args ...any) {
		t.Helper()
		if err := store.Pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scan(&book, `INSERT INTO books (name, base_currency) VALUES ('B', 'TWD') RETURNING id`)
	scan(&cash, `INSERT INTO accounts (book_id, class, name, commodity, is_cash) VALUES ($1, 'asset', 'Cash', 'TWD', true) RETURNING id`, book)
	scan(&food, `INSERT INTO accounts (book_id, class, name) VALUES ($1, 'expense', 'Food') RETURNING id`, book)
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO transactions (book_id, date, source) VALUES ($1, '2026-09-30', 'adjustment') RETURNING id`, book).Scan(&txn); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO postings (transaction_id, account_id, commodity, amount, base_amount)
	    VALUES ($1, $2, 'TWD', 100, 100), ($1, $3, 'TWD', -100, -100)`, txn, food, cash); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE books SET lock_date = '2026-09-30' WHERE id = $1`, book); err != nil {
		t.Fatal(err)
	}
	store.Close()

	if err := db.MigrateTo(dsn, 15); err != nil {
		t.Fatalf("down to 000015: %v", err)
	}
	store, err = db.Open(ctx, dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	var source string
	if err := store.Pool.QueryRow(ctx, `SELECT source FROM transactions WHERE id = $1`, txn).Scan(&source); err != nil || source != "manual" {
		t.Fatalf("source after down = %q (%v)", source, err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE books SET lock_date = NULL WHERE id = $1`, book); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE transactions SET source = 'adjustment' WHERE id = $1`, txn); err == nil {
		t.Fatal("000015 took an adjustment")
	}
	store.Close()
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
