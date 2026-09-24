package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cwchen-twn/rigel-ledger/migrations"
)

// Store owns the connection pool and hands out Queries, either directly for
// reads or inside WithTx for anything that writes.
type Store struct {
	Pool *pgxpool.Pool
	*Queries
}

// Open connects a pool to dsn (a postgres:// URL) and registers the
// shopspring/decimal codec so NUMERIC columns never pass through float64.
func Open(ctx context.Context, dsn string, maxConns int32) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool, Queries: New(pool)}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// WithTx runs fn in one database transaction. userID feeds the audit
// trigger through app.current_user; pass 0 for changes made outside the API.
// The deferred balance trigger fires at COMMIT, so its error is returned from
// here, not from the INSERT that caused it.
func (s *Store) WithTx(ctx context.Context, userID int64, fn func(q *Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if userID != 0 {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user', $1, true)", strconv.FormatInt(userID, 10)); err != nil {
			return err
		}
	}
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Migrate applies every embedded migration to dsn.
func Migrate(dsn string) error {
	src, err := iofs.New(migrations.MigrationsFiles, ".")
	if err != nil {
		return err
	}
	// golang-migrate's pgx v5 driver registers the pgx5:// scheme.
	m, err := migrate.NewWithSourceInstance("iofs", src, toMigrateURL(dsn))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// MigrateTo moves the schema to version (down or up). Only tests use it;
// production rolls back with a restore, never by running a down file.
func MigrateTo(dsn string, version uint) error {
	src, err := iofs.New(migrations.MigrationsFiles, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toMigrateURL(dsn))
	if err != nil {
		return err
	}
	defer m.Close()
	return m.Migrate(version)
}

func toMigrateURL(dsn string) string {
	for _, p := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(dsn, p) {
			return "pgx5://" + strings.TrimPrefix(dsn, p)
		}
	}
	return dsn
}
