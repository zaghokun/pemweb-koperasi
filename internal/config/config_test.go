package config

import (
	"errors"
	"strings"
	"testing"
)

func TestConfigLoadFrom_Success(t *testing.T) {
	validDSN := "koperasi_app:secret123@tcp(127.0.0.1:3306)/koperasi?parseTime=true&loc=UTC"
	env := map[string]string{
		"APP_ADDR":      "127.0.0.1:8080",
		"DATABASE_DSN":  validDSN,
		"COOKIE_SECURE": "false",
	}
	lookup := func(k string) string { return env[k] }

	cfg, err := LoadFrom(lookup)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if cfg.AppAddr != "127.0.0.1:8080" {
		t.Errorf("expected AppAddr 127.0.0.1:8080, got %s", cfg.AppAddr)
	}
	if cfg.DatabaseDSN != validDSN {
		t.Errorf("expected DatabaseDSN %s, got %s", validDSN, cfg.DatabaseDSN)
	}
	if cfg.CookieSecure != false {
		t.Errorf("expected CookieSecure false, got %v", cfg.CookieSecure)
	}
	if cfg.BusinessLoc == nil || cfg.BusinessLoc.String() != "Asia/Jakarta" {
		t.Errorf("expected BusinessLoc Asia/Jakarta, got %v", cfg.BusinessLoc)
	}

	// Test CookieSecure=true
	env["COOKIE_SECURE"] = "true"
	cfg, err = LoadFrom(lookup)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cfg.CookieSecure != true {
		t.Errorf("expected CookieSecure true, got %v", cfg.CookieSecure)
	}
}

func TestConfigLoadFrom_ValidationErrors(t *testing.T) {
	validDSN := "koperasi_app:secret123@tcp(127.0.0.1:3306)/koperasi?parseTime=true&loc=UTC"

	tests := []struct {
		name        string
		env         map[string]string
		expectedErr error
	}{
		{
			name:        "missing APP_ADDR",
			env:         map[string]string{"DATABASE_DSN": validDSN, "COOKIE_SECURE": "false"},
			expectedErr: ErrMissingAppAddr,
		},
		{
			name:        "invalid APP_ADDR without port",
			env:         map[string]string{"APP_ADDR": "localhost", "DATABASE_DSN": validDSN, "COOKIE_SECURE": "false"},
			expectedErr: ErrInvalidAppAddr,
		},
		{
			name:        "missing DATABASE_DSN",
			env:         map[string]string{"APP_ADDR": "127.0.0.1:8080", "COOKIE_SECURE": "false"},
			expectedErr: ErrMissingDatabaseDSN,
		},
		{
			name:        "invalid DATABASE_DSN format",
			env:         map[string]string{"APP_ADDR": "127.0.0.1:8080", "DATABASE_DSN": "://invalid-dsn", "COOKIE_SECURE": "false"},
			expectedErr: ErrInvalidDatabaseDSN,
		},
		{
			name:        "missing COOKIE_SECURE",
			env:         map[string]string{"APP_ADDR": "127.0.0.1:8080", "DATABASE_DSN": validDSN},
			expectedErr: ErrMissingCookieSecure,
		},
		{
			name:        "invalid COOKIE_SECURE value",
			env:         map[string]string{"APP_ADDR": "127.0.0.1:8080", "DATABASE_DSN": validDSN, "COOKIE_SECURE": "1"},
			expectedErr: ErrInvalidCookieSecure,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) string { return tc.env[k] }
			_, err := LoadFrom(lookup)
			if err == nil {
				t.Fatalf("expected error %v, got nil", tc.expectedErr)
			}
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("expected error %v, got %v", tc.expectedErr, err)
			}
			// Pastikan rahasia DSN tidak bocor dalam pesan error
			if strings.Contains(err.Error(), "secret123") {
				t.Errorf("error message leaked secret: %s", err.Error())
			}
		})
	}
}
