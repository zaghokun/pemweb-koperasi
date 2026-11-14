package domain

import "time"

// User merepresentasikan akun pengurus koperasi.
type User struct {
	ID           int64
	Name         string
	Username     string
	PasswordHash string
	IsActive     bool
	CreatedAt    time.Time
}

// Session merepresentasikan sesi pengurus atau formulir login di server.
type Session struct {
	ID        int64
	TokenHash [32]byte
	UserID    *int64
	CSRFToken string
	CreatedAt time.Time
	ExpiresAt time.Time
}
