package db_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/dbtest"
)

// These tests write SQL directly, bypassing internal/ledger, to prove the
// triggers hold the books together on their own.

type schemaFixture struct {
	ctx      context.Context
	store    *db.Store
	userID   int64
	bookID   int64
	cash     int64 // TWD asset
	expense  int64
	usdBank  int64
	group    int64 // placeholder
	otherBID int64
}

func setupSchema(t *testing.T) *schemaFixture {
	t.Helper()
	f := &schemaFixture{ctx: context.Background(), store: dbtest.New(t)}
	mustScan := func(dst *int64, sql string, args ...any) {
		t.Helper()
		if err := f.store.Pool.QueryRow(f.ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	mustScan(&f.userID, `INSERT INTO users (username, email, password_hash) VALUES ('alice', 'a@example.com', 'x') RETURNING id`)
	mustScan(&f.bookID, `INSERT INTO books (name, base_currency) VALUES ('B', 'TWD') RETURNING id`)
	mustScan(&f.otherBID, `INSERT INTO books (name, base_currency) VALUES ('Other', 'TWD') RETURNING id`)
	mustScan(&f.cash, `INSERT INTO accounts (book_id, class, name, commodity, is_cash) VALUES ($1, 'asset', 'Cash', 'TWD', true) RETURNING id`, f.bookID)
	mustScan(&f.usdBank, `INSERT INTO accounts (book_id, class, name, commodity) VALUES ($1, 'asset', 'USD bank', 'USD') RETURNING id`, f.bookID)
	mustScan(&f.expense, `INSERT INTO accounts (book_id, class, name) VALUES ($1, 'expense', 'Food') RETURNING id`, f.bookID)
	mustScan(&f.group, `INSERT INTO accounts (book_id, class, name, commodity, is_placeholder) VALUES ($1, 'asset', 'Banks', 'TWD', true) RETURNING id`, f.bookID)
	return f
}

// run executes fn in a transaction as the fixture user and returns the error
// from fn or, for deferred triggers, from COMMIT.
func (f *schemaFixture) run(fn func(tx pgx.Tx) error) error {
	tx, err := f.store.Pool.Begin(f.ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	if _, err := tx.Exec(f.ctx, "SELECT set_config('app.current_user', $1, true)", strconv.FormatInt(f.userID, 10)); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(f.ctx)
}

func (f *schemaFixture) newTxn(tx pgx.Tx, date string) (int64, error) {
	var id int64
	err := tx.QueryRow(f.ctx, `INSERT INTO transactions (book_id, date) VALUES ($1, $2) RETURNING id`, f.bookID, date).Scan(&id)
	return id, err
}

func post(ctx context.Context, tx pgx.Tx, txn, acct int64, commodity, amount, base string) error {
	_, err := tx.Exec(ctx, `INSERT INTO postings (transaction_id, account_id, commodity, amount, base_amount) VALUES ($1, $2, $3, $4, $5)`,
		txn, acct, commodity, amount, base)
	return err
}

func wantConstraint(t *testing.T, err error, name string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		t.Fatalf("error = %v, want constraint %s", err, name)
	}
	if pg.ConstraintName != name {
		t.Fatalf("constraint = %q (%s), want %q", pg.ConstraintName, pg.Message, name)
	}
}

func (f *schemaFixture) balancedTxn(t *testing.T, date string) int64 {
	t.Helper()
	var id int64
	if err := f.run(func(tx pgx.Tx) error {
		var err error
		if id, err = f.newTxn(tx, date); err != nil {
			return err
		}
		if err := post(f.ctx, tx, id, f.expense, "TWD", "100", "100"); err != nil {
			return err
		}
		return post(f.ctx, tx, id, f.cash, "TWD", "-100", "-100")
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBalanceTriggerOnInsert(t *testing.T) {
	f := setupSchema(t)
	f.balancedTxn(t, "2026-09-01")

	err := f.run(func(tx pgx.Tx) error {
		id, err := f.newTxn(tx, "2026-09-01")
		if err != nil {
			return err
		}
		if err := post(f.ctx, tx, id, f.expense, "TWD", "100", "100"); err != nil {
			return err
		}
		return post(f.ctx, tx, id, f.cash, "TWD", "-90", "-90")
	})
	wantConstraint(t, err, "transaction_balanced")

	err = f.run(func(tx pgx.Tx) error {
		_, err := f.newTxn(tx, "2026-09-01")
		return err
	})
	wantConstraint(t, err, "transaction_min_postings")
}

func TestBalanceTriggerOnUpdateAndDelete(t *testing.T) {
	f := setupSchema(t)
	id := f.balancedTxn(t, "2026-09-01")

	err := f.run(func(tx pgx.Tx) error {
		_, err := tx.Exec(f.ctx, `UPDATE postings SET amount = 50, base_amount = 50 WHERE transaction_id = $1 AND account_id = $2`, id, f.expense)
		return err
	})
	wantConstraint(t, err, "transaction_balanced")

	err = f.run(func(tx pgx.Tx) error {
		_, err := tx.Exec(f.ctx, `DELETE FROM postings WHERE transaction_id = $1 AND account_id = $2`, id, f.expense)
		return err
	})
	wantConstraint(t, err, "transaction_min_postings")

	// Deleting the whole transaction is fine: its postings go with it.
	if err := f.run(func(tx pgx.Tx) error {
		_, err := tx.Exec(f.ctx, `DELETE FROM transactions WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostingRules(t *testing.T) {
	f := setupSchema(t)
	cases := []struct {
		name       string
		acct       int64
		commodity  string
		amt, base  string
		constraint string
	}{
		{"commodity mismatch", f.cash, "USD", "10", "320", "posting_commodity"},
		{"placeholder", f.group, "TWD", "10", "10", "posting_account_placeholder"},
		{"base currency must equal amount", f.cash, "TWD", "10", "11", "posting_base_amount"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := f.run(func(tx pgx.Tx) error {
				id, err := f.newTxn(tx, "2026-09-01")
				if err != nil {
					return err
				}
				return post(f.ctx, tx, id, c.acct, c.commodity, c.amt, c.base)
			})
			wantConstraint(t, err, c.constraint)
		})
	}

	// An account from another book.
	var foreign int64
	if err := f.store.Pool.QueryRow(f.ctx, `INSERT INTO accounts (book_id, class, name, commodity) VALUES ($1, 'asset', 'X', 'TWD') RETURNING id`, f.otherBID).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	err := f.run(func(tx pgx.Tx) error {
		id, err := f.newTxn(tx, "2026-09-01")
		if err != nil {
			return err
		}
		return post(f.ctx, tx, id, foreign, "TWD", "10", "10")
	})
	wantConstraint(t, err, "posting_account_book")

	// A foreign-currency line balanced in base currency is accepted.
	if err := f.run(func(tx pgx.Tx) error {
		id, err := f.newTxn(tx, "2026-09-01")
		if err != nil {
			return err
		}
		if err := post(f.ctx, tx, id, f.expense, "USD", "10", "321"); err != nil {
			return err
		}
		return post(f.ctx, tx, id, f.usdBank, "USD", "-10", "-321")
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLockDateTrigger(t *testing.T) {
	f := setupSchema(t)
	id := f.balancedTxn(t, "2026-06-30")
	if _, err := f.store.Pool.Exec(f.ctx, `UPDATE books SET lock_date = '2026-06-30' WHERE id = $1`, f.bookID); err != nil {
		t.Fatal(err)
	}
	err := f.run(func(tx pgx.Tx) error {
		_, err := tx.Exec(f.ctx, `UPDATE transactions SET date = '2026-07-15' WHERE id = $1`, id)
		return err
	})
	wantConstraint(t, err, "book_locked")
	err = f.run(func(tx pgx.Tx) error {
		_, err := tx.Exec(f.ctx, `UPDATE postings SET memo = 'x' WHERE transaction_id = $1`, id)
		return err
	})
	wantConstraint(t, err, "book_locked")
	f.balancedTxn(t, "2026-07-01")
}

func TestAccountParentRules(t *testing.T) {
	f := setupSchema(t)
	_, err := f.store.Pool.Exec(f.ctx, `INSERT INTO accounts (book_id, parent_id, class, name) VALUES ($1, $2, 'expense', 'Wrong')`, f.bookID, f.group)
	wantConstraint(t, err, "account_parent")

	var child int64
	if err := f.store.Pool.QueryRow(f.ctx, `INSERT INTO accounts (book_id, parent_id, class, name, commodity) VALUES ($1, $2, 'asset', 'Child', 'TWD') RETURNING id`, f.bookID, f.group).Scan(&child); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Pool.Exec(f.ctx, `UPDATE accounts SET parent_id = $1 WHERE id = $2`, child, f.group)
	wantConstraint(t, err, "account_parent")
}

func TestAuditTrail(t *testing.T) {
	f := setupSchema(t)
	id := f.balancedTxn(t, "2026-09-01")
	var n int
	var by *int64
	if err := f.store.Pool.QueryRow(f.ctx,
		`SELECT count(*), max(changed_by) FROM audit_log WHERE book_id = $1 AND (table_name = 'transactions' AND row_id = $2 OR table_name = 'postings')`,
		f.bookID, id).Scan(&n, &by); err != nil {
		t.Fatal(err)
	}
	if n != 3 || by == nil || *by != f.userID {
		t.Fatalf("audit rows = %d by %v, want 3 by %d", n, by, f.userID)
	}
}

// A rate inside a locked period is frozen for books that use its currencies,
// so a corrected old rate cannot move a closed balance sheet.
func TestPriceLockTrigger(t *testing.T) {
	f := setupSchema(t)
	if _, err := f.store.Pool.Exec(f.ctx, `INSERT INTO prices (commodity, quote, date, rate) VALUES ('USD', 'TWD', '2026-06-01', 32)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Pool.Exec(f.ctx, `UPDATE books SET lock_date = '2026-06-30' WHERE id = $1`, f.bookID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.Pool.Exec(f.ctx, `UPDATE prices SET rate = 33 WHERE commodity = 'USD' AND quote = 'TWD'`)
	wantConstraint(t, err, "book_locked")
	_, err = f.store.Pool.Exec(f.ctx, `INSERT INTO prices (commodity, quote, date, rate) VALUES ('USD', 'TWD', '2026-05-01', 31)`)
	wantConstraint(t, err, "book_locked")
	_, err = f.store.Pool.Exec(f.ctx, `DELETE FROM prices WHERE commodity = 'USD'`)
	wantConstraint(t, err, "book_locked")

	// After the lock date, or for currencies no locked book uses, rates are free.
	for _, q := range []string{
		`INSERT INTO prices (commodity, quote, date, rate) VALUES ('USD', 'TWD', '2026-07-01', 32.5)`,
		`INSERT INTO prices (commodity, quote, date, rate) VALUES ('EUR', 'JPY', '2026-01-01', 160)`,
	} {
		if _, err := f.store.Pool.Exec(f.ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// Attachments stay in their book, keep their bytes out of the audit log, and
// go when nothing points at them any more (000009).
func TestAttachmentTriggers(t *testing.T) {
	f := setupSchema(t)
	txn := f.balancedTxn(t, "2026-09-01")
	var other, oCash, oFood int64
	_ = f.store.Pool.QueryRow(f.ctx, `INSERT INTO accounts (book_id, class, name, commodity) VALUES ($1, 'asset', 'Cash', 'TWD') RETURNING id`, f.otherBID).Scan(&oCash)
	_ = f.store.Pool.QueryRow(f.ctx, `INSERT INTO accounts (book_id, class, name) VALUES ($1, 'expense', 'Food') RETURNING id`, f.otherBID).Scan(&oFood)
	if err := f.run(func(tx pgx.Tx) error {
		if err := tx.QueryRow(f.ctx, `INSERT INTO transactions (book_id, date) VALUES ($1, '2026-09-01') RETURNING id`, f.otherBID).Scan(&other); err != nil {
			return err
		}
		if err := post(f.ctx, tx, other, oFood, "TWD", "100", "100"); err != nil {
			return err
		}
		return post(f.ctx, tx, other, oCash, "TWD", "-100", "-100")
	}); err != nil {
		t.Fatal(err)
	}
	newFile := func(tx pgx.Tx, content string) (int64, error) {
		var id int64
		err := tx.QueryRow(f.ctx, `INSERT INTO attachments (book_id, sha256, filename, mime, size, bytes)
			VALUES ($1, sha256($2::BYTEA), 'receipt.jpg', 'image/jpeg', octet_length($2::BYTEA), $2::BYTEA) RETURNING id`,
			f.bookID, []byte(content)).Scan(&id)
		return id, err
	}
	link := func(tx pgx.Tx, book, txn, file int64) error {
		_, err := tx.Exec(f.ctx, `INSERT INTO transaction_attachments (book_id, transaction_id, attachment_id) VALUES ($1, $2, $3)`, book, txn, file)
		return err
	}

	// Not an image or a PDF: refused, whatever the client calls it.
	wantConstraint(t, f.run(func(tx pgx.Tx) error {
		_, err := tx.Exec(f.ctx, `INSERT INTO attachments (book_id, sha256, filename, mime, size, bytes)
			VALUES ($1, sha256('x'::BYTEA), 'a.svg', 'image/svg+xml', 1, 'x'::BYTEA)`, f.bookID)
		return err
	}), "attachments_mime_check")

	// Another book's transaction cannot take this book's file.
	wantConstraint(t, f.run(func(tx pgx.Tx) error {
		file, err := newFile(tx, "receipt")
		if err != nil {
			return err
		}
		return link(tx, f.otherBID, other, file)
	}), "attachment_book")
	wantConstraint(t, f.run(func(tx pgx.Tx) error {
		file, err := newFile(tx, "receipt")
		if err != nil {
			return err
		}
		return link(tx, f.bookID, other, file)
	}), "attachment_book")

	var file int64
	if err := f.run(func(tx pgx.Tx) error {
		var err error
		if file, err = newFile(tx, "the receipt's bytes"); err != nil {
			return err
		}
		return link(tx, f.bookID, txn, file)
	}); err != nil {
		t.Fatal(err)
	}
	var audited string
	if err := f.store.Pool.QueryRow(f.ctx, `SELECT new_values::TEXT FROM audit_log WHERE table_name = 'attachments' AND row_id = $1`, file).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(audited, "bytes") || !strings.Contains(audited, "receipt.jpg") {
		t.Fatalf("audited %s: the metadata, never the bytes", audited)
	}

	// A staged row still holding the file keeps it when its last link goes.
	var src, batch, row int64
	if err := f.store.Pool.QueryRow(f.ctx, `INSERT INTO source_accounts (book_id, connector, external_id) VALUES ($1, 'csv', 'a') RETURNING id`, f.bookID).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Pool.QueryRow(f.ctx, `INSERT INTO import_batches (book_id, connector) VALUES ($1, 'csv') RETURNING id`, f.bookID).Scan(&batch); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Pool.QueryRow(f.ctx, `INSERT INTO import_rows (book_id, batch_id, source_account_id, kind, external_id, date, amount, currency, attachment_id)
		VALUES ($1, $2, $3, 'transaction', 'csv:1', '2026-09-01', -100, 'TWD', $4) RETURNING id`, f.bookID, batch, src, file).Scan(&row); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		var n int
		_ = f.store.Pool.QueryRow(f.ctx, `SELECT count(*) FROM attachments WHERE id = $1`, file).Scan(&n)
		return n
	}
	if err := f.run(func(tx pgx.Tx) error {
		_, err := tx.Exec(f.ctx, `DELETE FROM transactions WHERE id = $1`, txn)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatal("the file went while a staged row still held it")
	}
	if _, err := f.store.Pool.Exec(f.ctx, `DELETE FROM import_rows WHERE id = $1`, row); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatal("a file nothing points at was kept")
	}
}

// Holding and trade rows carry a security and units; other rows carry
// neither (000010). Go checks first; this holds for any writer.
func TestImportRowShape(t *testing.T) {
	f := setupSchema(t)
	var src, batch int64
	_ = f.store.Pool.QueryRow(f.ctx, `INSERT INTO source_accounts (book_id, connector, external_id, kind) VALUES ($1, 'tw-tdcc', 'b', 'brokerage') RETURNING id`, f.bookID).Scan(&src)
	_ = f.store.Pool.QueryRow(f.ctx, `INSERT INTO import_batches (book_id, connector) VALUES ($1, 'tw-tdcc') RETURNING id`, f.bookID).Scan(&batch)
	if _, err := f.store.Pool.Exec(f.ctx, `INSERT INTO commodities (code, kind, name, decimals, quote_currency) VALUES ('XTAI:2330', 'security', 'TSMC', 0, 'TWD')`); err != nil {
		t.Fatal(err)
	}
	row := func(id, kind, amount string, security, units, cash any) error {
		_, err := f.store.Pool.Exec(f.ctx, `INSERT INTO import_rows (book_id, batch_id, source_account_id, kind, external_id, date, amount, currency, security, units, cash)
			VALUES ($1, $2, $3, $4, $5, '2026-09-01', $6, 'TWD', $7, $8, $9)`, f.bookID, batch, src, kind, id, amount, security, units, cash)
		return err
	}
	for _, c := range []struct {
		name, kind, amount string
		security, units    any
		cash               any
	}{
		{"a trade moving nothing", "trade", "0", "XTAI:2330", "0", nil},
		{"a trade without its security", "trade", "0", nil, "10", nil},
		{"a trade with an amount", "trade", "-100", "XTAI:2330", "10", "-100"},
		{"a negative holding", "holding", "0", "XTAI:2330", "-1", nil},
		{"a holding with cash", "holding", "0", "XTAI:2330", "10", "5"},
		{"a transaction with units", "transaction", "-100", nil, "10", nil},
	} {
		if err := row("x:"+c.name, c.kind, c.amount, c.security, c.units, c.cash); err == nil {
			t.Errorf("%s was accepted", c.name)
		} else {
			wantConstraint(t, err, "import_rows_shape")
		}
	}
	if err := row("x:ok", "trade", "0", "XTAI:2330", "1000", nil); err != nil {
		t.Fatalf("a trade without cash yet: %v", err)
	}
}
