package database

import (
	"context"
	"datastar-go/config"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func New(ctx context.Context) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(config.Global.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}
	poolCfg.MaxConns = int32(config.Global.DBMaxConns)
	poolCfg.MinConns = int32(config.Global.DBMinConns)
	poolCfg.MaxConnIdleTime = config.Global.DBIdleTimeout
	poolCfg.ConnConfig.ConnectTimeout = config.Global.DBConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}
