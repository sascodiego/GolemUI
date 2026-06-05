package db

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

type DB struct {
	CorePool     *pgxpool.Pool
	BusinessPool *pgxpool.Pool
}

func InitDB(ctx context.Context, coreCfg Config, bizCfg Config) (*DB, error) {
	coreConnStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		coreCfg.User, coreCfg.Password, coreCfg.Host, coreCfg.Port, coreCfg.Database)

	coreConfig, err := pgxpool.ParseConfig(coreConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse core config: %w", err)
	}

	corePool, err := pgxpool.NewWithConfig(ctx, coreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create core pool: %w", err)
	}

	// Ping core database to check health/readiness
	if err := corePool.Ping(ctx); err != nil {
		corePool.Close()
		return nil, fmt.Errorf("failed to ping core database: %w", err)
	}

	bizConnStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		bizCfg.User, bizCfg.Password, bizCfg.Host, bizCfg.Port, bizCfg.Database)

	bizConfig, err := pgxpool.ParseConfig(bizConnStr)
	if err != nil {
		corePool.Close()
		return nil, fmt.Errorf("failed to parse business config: %w", err)
	}

	bizPool, err := pgxpool.NewWithConfig(ctx, bizConfig)
	if err != nil {
		corePool.Close()
		return nil, fmt.Errorf("failed to create business pool: %w", err)
	}

	// Ping business database to check health/readiness
	if err := bizPool.Ping(ctx); err != nil {
		corePool.Close()
		bizPool.Close()
		return nil, fmt.Errorf("failed to ping business database: %w", err)
	}

	return &DB{
		CorePool:     corePool,
		BusinessPool: bizPool,
	}, nil
}

func (db *DB) Close() {
	if db.CorePool != nil {
		db.CorePool.Close()
	}
	if db.BusinessPool != nil {
		db.BusinessPool.Close()
	}
}
