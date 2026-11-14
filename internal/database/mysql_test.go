package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpen_InvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := Open(ctx, "://bad-dsn")
	if err == nil {
		t.Fatal("expected error on invalid DSN, got nil")
	}
	if !errors.Is(err, ErrInvalidDSN) {
		t.Errorf("expected ErrInvalidDSN, got %v", err)
	}
}

func TestOpen_ConnectionFailureDoesNotLeakCredentials(t *testing.T) {
	ctx := context.Background()
	// Gunakan port tertutup atau host tak terjangkau dengan kredensial palsu
	fakeDSN := "testuser:supersecretpassword@tcp(127.0.0.1:54321)/koperasi_nonexistent?timeout=200ms"
	poolCfg := DefaultPoolConfig()
	poolCfg.PingTimeout = 300 * time.Millisecond

	_, err := OpenWithConfig(ctx, fakeDSN, poolCfg)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "supersecretpassword") {
		t.Errorf("connection error leaked sensitive password: %s", errMsg)
	}
}

func TestOpen_LiveDatabaseIfConfigured(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set, skipping live database test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("failed to execute test query: %v", err)
	}
	if one != 1 {
		t.Errorf("expected 1, got %d", one)
	}

	// Verifikasi session time_zone adalah '+00:00'
	var tz string
	if err := db.QueryRowContext(ctx, "SELECT @@session.time_zone").Scan(&tz); err != nil {
		t.Fatalf("failed to query session time_zone: %v", err)
	}
	if tz != "+00:00" {
		t.Errorf("expected session time_zone '+00:00', got %q", tz)
	}
}
