package config

import (
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrMissingAppAddr      = errors.New("APP_ADDR is required")
	ErrInvalidAppAddr      = errors.New("APP_ADDR must be a valid host:port or :port address")
	ErrMissingDatabaseDSN  = errors.New("DATABASE_DSN is required")
	ErrInvalidDatabaseDSN  = errors.New("invalid DATABASE_DSN (credentials omitted)")
	ErrMissingCookieSecure = errors.New("COOKIE_SECURE is required")
	ErrInvalidCookieSecure = errors.New("COOKIE_SECURE must be 'true' or 'false'")
)

// Config menyimpan konfigurasi runtime aplikasi koperasi.
type Config struct {
	AppAddr      string
	DatabaseDSN  string
	CookieSecure bool
	BusinessLoc  *time.Location
}

// Load membaca konfigurasi dari environment variable proses.
func Load() (*Config, error) {
	return LoadFrom(os.Getenv)
}

// LoadFrom membaca dan memvalidasi konfigurasi menggunakan fungsi pencarian env yang diberikan.
func LoadFrom(lookup func(string) string) (*Config, error) {
	appAddr := strings.TrimSpace(lookup("APP_ADDR"))
	if appAddr == "" {
		return nil, ErrMissingAppAddr
	}
	if err := validateAppAddr(appAddr); err != nil {
		return nil, err
	}

	rawDSN := strings.TrimSpace(lookup("DATABASE_DSN"))
	if rawDSN == "" {
		return nil, ErrMissingDatabaseDSN
	}
	// Parse DSN untuk memastikan format driver MySQL valid tanpa membocorkan kredensial jika gagal.
	if _, err := mysql.ParseDSN(rawDSN); err != nil {
		return nil, ErrInvalidDatabaseDSN
	}

	rawCookieSecure := strings.TrimSpace(lookup("COOKIE_SECURE"))
	if rawCookieSecure == "" {
		return nil, ErrMissingCookieSecure
	}
	var cookieSecure bool
	switch rawCookieSecure {
	case "true":
		cookieSecure = true
	case "false":
		cookieSecure = false
	default:
		return nil, ErrInvalidCookieSecure
	}

	// Lokasi waktu bisnis Asia/Jakarta untuk representasi tanggal bisnis.
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("Asia/Jakarta", 7*3600)
	}

	return &Config{
		AppAddr:      appAddr,
		DatabaseDSN:  rawDSN,
		CookieSecure: cookieSecure,
		BusinessLoc:  loc,
	}, nil
}

func validateAppAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ErrInvalidAppAddr
	}
	if port == "" {
		return ErrInvalidAppAddr
	}
	_ = host
	return nil
}
