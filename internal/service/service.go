package service

import (
	"context"
	"errors"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("username atau password salah")
	ErrSessionExpired     = errors.New("sesi telah kedaluwarsa")
	ErrUserInactive       = errors.New("akun pengurus tidak aktif")
	ErrUnauthorized       = errors.New("akses ditolak")
	ErrInvalidCSRF        = errors.New("token CSRF tidak sah")
)

type UserRepo interface {
	GetByID(ctx context.Context, db repository.DBTX, id int64) (*domain.User, error)
	GetByUsername(ctx context.Context, db repository.DBTX, username string) (*domain.User, error)
	Create(ctx context.Context, db repository.DBTX, user *domain.User) error
}

type SessionRepo interface {
	Create(ctx context.Context, db repository.DBTX, s *domain.Session) error
	GetByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) (*domain.Session, error)
	DeleteByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) error
	DeleteExpired(ctx context.Context, db repository.DBTX, now time.Time) error
}
