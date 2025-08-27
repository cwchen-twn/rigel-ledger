package internal

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/cwc1222/rigelledger/migrations"
)

type Postgres struct {
	db     *sqlx.DB
	dsn    string
	logger *slog.Logger
}

const (
	defaultTimeout = 5 * time.Second
)

func NewPostgres(cfg *Config, logger *slog.Logger) (*Postgres, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	logger.Info("Connecting to postgres", "host", cfg.PgHost, "port", cfg.PgPort, "dbname", cfg.PgDbname)

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.PgUser,
		cfg.PgPassword,
		cfg.PgHost,
		cfg.PgPort,
		cfg.PgDbname,
	)
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.PgMaxOpenConns)
	db.SetMaxIdleConns(cfg.PgMaxIdleConns)
	db.SetConnMaxLifetime(cfg.PgMaxConnLifetime)
	db.SetConnMaxIdleTime(cfg.PgMaxConnIdleTime)

	logger.Info("Connected to postgres", "host", cfg.PgHost, "port", cfg.PgPort, "dbname", cfg.PgDbname)

	pg := &Postgres{db: db, dsn: dsn, logger: logger}

	if err := pg.Migrate(migrations.MigrationsFiles); err != nil {
		return nil, err
	}

	return pg, nil
}

func (p *Postgres) Close() error {
	p.logger.Info("Closing postgres connection pool")
	return p.db.Close()
}

func (p *Postgres) Migrate(fsys fs.FS) error {
	p.logger.Info("Start to apply migrations")

	iofsSource, err := iofs.New(fsys, ".")
	if err != nil {
		p.logger.Error("Failed to create iofs source", "error", err)
		return err
	}

	m, err := migrate.NewWithSourceInstance("iofs", iofsSource, p.dsn)
	if err != nil {
		p.logger.Error("Failed to create migrate instance", "error", err)
		return err
	}

	err = m.Up()
	if err == migrate.ErrNoChange {
		p.logger.Info("No migrations to apply")
		return nil
	}

	if err != nil {
		p.logger.Error("Failed to migrate", "error", err)
		return err
	}

	return nil
}

func (p *Postgres) GetDB() *sqlx.DB {
	return p.db
}

/*
func NewPgPool(cfg *Config) (*pgxpool.Pool, error) {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable"+
			"&pool_max_conns=%d"+
			"&pool_max_conn_lifetime=%s"+
			"&pool_min_conns=%d"+
			"&pool_max_conn_idle_time=%s"+
			"&pool_max_conn_lifetime_jitter=%s"+
			"&pool_health_check_period=%s",
		cfg.PgUser,
		cfg.PgPassword,
		cfg.PgHost,
		cfg.PgPort,
		cfg.PgDbname,
		cfg.PgMaxConns,
		cfg.PgMaxConnLifetime,
		cfg.PgMinConns,
		cfg.PgMaxConnIdleTime,
		cfg.PgMaxConnLifetimeJitter,
		cfg.PgHealthCheckPeriod,
	)
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	return pool, nil
}
*/
