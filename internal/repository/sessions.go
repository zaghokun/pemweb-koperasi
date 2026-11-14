package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"koperasi/internal/domain"
)

type SessionRepository struct{}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{}
}

// Create menyimpan record sesi baru ke database.
func (r *SessionRepository) Create(ctx context.Context, db DBTX, s *domain.Session) error {
	const query = `
		INSERT INTO sessions (token_hash, user_id, csrf_token, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`
	res, err := db.ExecContext(ctx, query,
		s.TokenHash[:],
		s.UserID,
		s.CSRFToken,
		s.CreatedAt.UTC(),
		s.ExpiresAt.UTC(),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	return nil
}

// GetByTokenHash mencari sesi berdasarkan hash token (SHA-256 32 byte).
func (r *SessionRepository) GetByTokenHash(ctx context.Context, db DBTX, tokenHash [32]byte) (*domain.Session, error) {
	const query = `
		SELECT id, token_hash, user_id, csrf_token, created_at, expires_at
		FROM sessions
		WHERE token_hash = ?
	`
	row := db.QueryRowContext(ctx, query, tokenHash[:])

	var s domain.Session
	var tokenHashBytes []byte
	var userID sql.NullInt64

	err := row.Scan(
		&s.ID,
		&tokenHashBytes,
		&userID,
		&s.CSRFToken,
		&s.CreatedAt,
		&s.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	copy(s.TokenHash[:], tokenHashBytes)
	if userID.Valid {
		uid := userID.Int64
		s.UserID = &uid
	}

	return &s, nil
}

// DeleteByTokenHash menghapus sesi berdasarkan hash token.
func (r *SessionRepository) DeleteByTokenHash(ctx context.Context, db DBTX, tokenHash [32]byte) error {
	const query = `DELETE FROM sessions WHERE token_hash = ?`
	_, err := db.ExecContext(ctx, query, tokenHash[:])
	return err
}

// DeleteExpired menghapus sesi-sesi yang telah kedaluwarsa sebelum waktu `now`.
func (r *SessionRepository) DeleteExpired(ctx context.Context, db DBTX, now time.Time) error {
	const query = `DELETE FROM sessions WHERE expires_at <= ?`
	_, err := db.ExecContext(ctx, query, now.UTC())
	return err
}
