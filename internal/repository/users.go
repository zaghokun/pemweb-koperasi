package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"koperasi/internal/domain"
)

var (
	ErrNotFound = errors.New("record not found")
)

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

// GetByID mengambil data pengguna berdasarkan ID.
func (r *UserRepository) GetByID(ctx context.Context, db DBTX, id int64) (*domain.User, error) {
	const query = `
		SELECT id, name, username, password_hash, is_active, created_at
		FROM users
		WHERE id = ?
	`
	row := db.QueryRowContext(ctx, query, id)
	return scanUser(row)
}

// GetByUsername mengambil data pengguna berdasarkan username (dinormalisasi ke huruf kecil).
func (r *UserRepository) GetByUsername(ctx context.Context, db DBTX, username string) (*domain.User, error) {
	const query = `
		SELECT id, name, username, password_hash, is_active, created_at
		FROM users
		WHERE username = ?
	`
	normalized := strings.ToLower(strings.TrimSpace(username))
	row := db.QueryRowContext(ctx, query, normalized)
	return scanUser(row)
}

// Create menambahkan pengguna baru ke tabel users.
func (r *UserRepository) Create(ctx context.Context, db DBTX, user *domain.User) error {
	const query = `
		INSERT INTO users (name, username, password_hash, is_active, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	normalized := strings.ToLower(strings.TrimSpace(user.Username))
	res, err := db.ExecContext(ctx, query,
		user.Name,
		normalized,
		user.PasswordHash,
		user.IsActive,
		user.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	user.Username = normalized
	return nil
}

func scanUser(scanner interface{ Scan(dest ...any) error }) (*domain.User, error) {
	var u domain.User
	var isActiveInt int
	err := scanner.Scan(
		&u.ID,
		&u.Name,
		&u.Username,
		&u.PasswordHash,
		&isActiveInt,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.IsActive = (isActiveInt == 1)
	return &u, nil
}
