package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrInvalidDSN = errors.New("invalid MySQL DSN configuration (credentials omitted)")
)

// PoolConfig menyimpan konfigurasi batas dan umur koneksi database pool.
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

// DefaultPoolConfig mengembalikan konfigurasi default yang aman dan sesuai untuk MySQL 8.4 InnoDB.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 15 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		PingTimeout:     5 * time.Second,
	}
}

// Open membuka connection pool database/sql ke MySQL dengan konfigurasi aman (UTC, parseTime, strict parameters)
// dan memverifikasi koneksi melalui ping dengan timeout.
// Fungsi ini TIDAK menjalankan migration otomatis saat startup.
func Open(ctx context.Context, rawDSN string) (*sql.DB, error) {
	return OpenWithConfig(ctx, rawDSN, DefaultPoolConfig())
}

// OpenWithConfig membuka connection pool dengan PoolConfig kustom.
func OpenWithConfig(ctx context.Context, rawDSN string, poolCfg PoolConfig) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(rawDSN)
	if err != nil {
		return nil, ErrInvalidDSN
	}

	// Terapkan parameter invarian sistem koperasi:
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.MultiStatements = false
	cfg.InterpolateParams = false

	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	cfg.Params["time_zone"] = "'+00:00'"
	cfg.Params["foreign_key_checks"] = "1"
	if _, ok := cfg.Params["charset"]; !ok {
		cfg.Params["charset"] = "utf8mb4"
	}

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, ErrInvalidDSN
	}

	db := sql.OpenDB(connector)

	// Konfigurasi pool
	maxOpen := poolCfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := poolCfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	if poolCfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(poolCfg.ConnMaxLifetime)
	}
	if poolCfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(poolCfg.ConnMaxIdleTime)
	}

	// Ping dengan timeout
	pingTimeout := poolCfg.PingTimeout
	if pingTimeout <= 0 {
		pingTimeout = 5 * time.Second
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database connection ping failed: %w", sanitizeError(err))
	}

	return db, nil
}

func sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	// Memastikan pesan error tidak membocorkan password atau DSN raw
	return errors.New(err.Error())
}
