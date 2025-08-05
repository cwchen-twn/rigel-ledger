package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
