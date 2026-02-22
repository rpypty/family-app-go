package db

import (
	"fmt"
	"time"

	"family-app-go/internal/config"
	"family-app-go/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
)

func NewPostgres(log logger.Logger, cfg config.DBConfig) (*gorm.DB, error) {
	if cfg.DSN != "" {
		log.Info("db: connecting using DSN")
	} else {
		log.Info(
			"db: connecting to postgres",
			"host",
			cfg.Host,
			"port",
			cfg.Port,
			"dbname",
			cfg.Name,
			"sslmode",
			cfg.SSLMode,
		)
	}

	dsn := cfg.GetDSN()
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("db handle: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen == 0 {
		maxOpen = defaultMaxOpenConns
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle == 0 {
		maxIdle = defaultMaxIdleConns
	}
	connMaxLifetime := cfg.ConnMaxLifetime
	if connMaxLifetime == 0 {
		connMaxLifetime = defaultConnMaxLifetime
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}

	log.Info("db: connected")
	return gormDB, nil
}
