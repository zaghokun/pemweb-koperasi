package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"time"

	"golang.org/x/crypto/bcrypt"

	"koperasi/internal/domain"
)

const (
	SessionDuration         = 8 * time.Hour
	PreLoginSessionDuration = 1 * time.Hour
)

type AuthService struct {
	db          *sql.DB
	userRepo    UserRepo
	sessionRepo SessionRepo
}

func NewAuthService(db *sql.DB, userRepo UserRepo, sessionRepo SessionRepo) *AuthService {
	return &AuthService{
		db:          db,
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
	}
}

// CreatePreLoginSession membuat sesi anonim untuk form login dan CSRF token awal.
func (s *AuthService) CreatePreLoginSession(ctx context.Context) (string, string, error) {
	token, tokenHash, err := generateToken()
	if err != nil {
		return "", "", err
	}

	csrf, err := generateRandomHex(32)
	if err != nil {
		return "", "", err
	}

	now := time.Now().UTC()
	sess := &domain.Session{
		TokenHash: tokenHash,
		UserID:    nil,
		CSRFToken: csrf,
		CreatedAt: now,
		ExpiresAt: now.Add(PreLoginSessionDuration),
	}

	if err := s.sessionRepo.Create(ctx, s.db, sess); err != nil {
		return "", "", err
	}

	return token, csrf, nil
}

// Login memvalidasi kredensial pengguna, merotasi sesi lama (mencegah session fixation),
// dan menerbitkan sesi baru 8 jam.
func (s *AuthService) Login(ctx context.Context, oldToken string, username, password string) (string, *domain.User, string, error) {
	user, err := s.userRepo.GetByUsername(ctx, s.db, username)
	if err != nil {
		return "", nil, "", ErrInvalidCredentials
	}

	// Bandingkan password bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, "", ErrInvalidCredentials
	}

	if !user.IsActive {
		return "", nil, "", ErrUserInactive
	}

	// Hapus sesi lama jika ada (rotasi sesi)
	if oldToken != "" {
		oldHash := HashToken(oldToken)
		_ = s.sessionRepo.DeleteByTokenHash(ctx, s.db, oldHash)
	}

	// Terbitkan sesi terotasi baru
	newToken, newTokenHash, err := generateToken()
	if err != nil {
		return "", nil, "", err
	}

	newCSRF, err := generateRandomHex(32)
	if err != nil {
		return "", nil, "", err
	}

	now := time.Now().UTC()
	newSess := &domain.Session{
		TokenHash: newTokenHash,
		UserID:    &user.ID,
		CSRFToken: newCSRF,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionDuration),
	}

	if err := s.sessionRepo.Create(ctx, s.db, newSess); err != nil {
		return "", nil, "", err
	}

	return newToken, user, newCSRF, nil
}

// ValidateSession memvalidasi token sesi dari cookie, memeriksa masa berlaku dan status aktif pengguna.
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*domain.Session, *domain.User, error) {
	if token == "" {
		return nil, nil, ErrUnauthorized
	}

	tokenHash := HashToken(token)
	sess, err := s.sessionRepo.GetByTokenHash(ctx, s.db, tokenHash)
	if err != nil {
		return nil, nil, ErrUnauthorized
	}

	now := time.Now().UTC()
	if now.After(sess.ExpiresAt) {
		_ = s.sessionRepo.DeleteByTokenHash(ctx, s.db, tokenHash)
		return nil, nil, ErrSessionExpired
	}

	if sess.UserID == nil {
		return sess, nil, nil
	}

	user, err := s.userRepo.GetByID(ctx, s.db, *sess.UserID)
	if err != nil {
		return nil, nil, ErrUnauthorized
	}

	if !user.IsActive {
		return nil, nil, ErrUserInactive
	}

	return sess, user, nil
}

// Logout mencabut sesi yang aktif di database.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	tokenHash := HashToken(token)
	return s.sessionRepo.DeleteByTokenHash(ctx, s.db, tokenHash)
}

// VerifyCSRF memverifikasi kecocokan token CSRF secara constant time.
func VerifyCSRF(sess *domain.Session, submittedToken string) bool {
	if sess == nil || submittedToken == "" || sess.CSRFToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sess.CSRFToken), []byte(submittedToken)) == 1
}

// HashToken menghitung SHA-256 dari token sesi mentah.
func HashToken(token string) [32]byte {
	return sha256.Sum256([]byte(token))
}

func generateToken() (string, [32]byte, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", [32]byte{}, err
	}
	tokenStr := hex.EncodeToString(b[:])
	tokenHash := HashToken(tokenStr)
	return tokenStr, tokenHash, nil
}

func generateRandomHex(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
