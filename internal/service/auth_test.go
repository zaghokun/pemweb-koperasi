package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
)

type mockUserRepo struct {
	usersByID       map[int64]*domain.User
	usersByUsername map[string]*domain.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		usersByID:       make(map[int64]*domain.User),
		usersByUsername: make(map[string]*domain.User),
	}
}

func (m *mockUserRepo) GetByID(ctx context.Context, db repository.DBTX, id int64) (*domain.User, error) {
	u, ok := m.usersByID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetByUsername(ctx context.Context, db repository.DBTX, username string) (*domain.User, error) {
	u, ok := m.usersByUsername[username]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepo) Create(ctx context.Context, db repository.DBTX, user *domain.User) error {
	m.usersByID[user.ID] = user
	m.usersByUsername[user.Username] = user
	return nil
}

type mockSessionRepo struct {
	sessions map[[32]byte]*domain.Session
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{
		sessions: make(map[[32]byte]*domain.Session),
	}
}

func (m *mockSessionRepo) Create(ctx context.Context, db repository.DBTX, s *domain.Session) error {
	m.sessions[s.TokenHash] = s
	return nil
}

func (m *mockSessionRepo) GetByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) (*domain.Session, error) {
	s, ok := m.sessions[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return s, nil
}

func (m *mockSessionRepo) DeleteByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) error {
	delete(m.sessions, tokenHash)
	return nil
}

func (m *mockSessionRepo) DeleteExpired(ctx context.Context, db repository.DBTX, now time.Time) error {
	for k, s := range m.sessions {
		if now.After(s.ExpiresAt) {
			delete(m.sessions, k)
		}
	}
	return nil
}

func TestAuthService_FullFlow(t *testing.T) {
	ctx := context.Background()
	userRepo := newMockUserRepo()
	sessRepo := newMockSessionRepo()
	authSvc := NewAuthService(nil, userRepo, sessRepo)

	// Seed satu user pengurus
	pwHash, err := bcrypt.GenerateFromPassword([]byte("rahasia123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	user := &domain.User{
		ID:           1,
		Name:         "Pengurus Satu",
		Username:     "pengurus1",
		PasswordHash: string(pwHash),
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}
	_ = userRepo.Create(ctx, nil, user)

	// 1. Buat sesi pra-login
	preToken, csrf, err := authSvc.CreatePreLoginSession(ctx)
	if err != nil {
		t.Fatalf("failed to create pre-login session: %v", err)
	}
	if preToken == "" || csrf == "" {
		t.Fatal("empty pre-login token or csrf")
	}

	// 2. Login dengan kredensial salah
	_, _, _, err = authSvc.Login(ctx, preToken, "pengurus1", "salah_password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// 3. Login dengan kredensial benar
	loginToken, loggedUser, newCSRF, err := authSvc.Login(ctx, preToken, "pengurus1", "rahasia123")
	if err != nil {
		t.Fatalf("expected successful login, got %v", err)
	}
	if loggedUser.ID != 1 {
		t.Errorf("expected user ID 1, got %d", loggedUser.ID)
	}
	if loginToken == preToken {
		t.Error("expected rotated token, but got same token as pre-login")
	}

	// Pastikan sesi lama telah dihapus
	oldHash := HashToken(preToken)
	if _, ok := sessRepo.sessions[oldHash]; ok {
		t.Error("old session was not deleted upon login rotation")
	}

	// 4. Validasi sesi login
	sess, u, err := authSvc.ValidateSession(ctx, loginToken)
	if err != nil {
		t.Fatalf("expected valid session, got error: %v", err)
	}
	if u == nil || u.ID != 1 {
		t.Errorf("expected user ID 1, got %v", u)
	}

	// 5. Verifikasi CSRF
	if !VerifyCSRF(sess, newCSRF) {
		t.Error("expected CSRF verification to pass")
	}
	if VerifyCSRF(sess, "invalid_csrf_token") {
		t.Error("expected CSRF verification to fail on bad token")
	}

	// 6. Logout
	if err := authSvc.Logout(ctx, loginToken); err != nil {
		t.Fatalf("failed to logout: %v", err)
	}
	_, _, err = authSvc.ValidateSession(ctx, loginToken)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized after logout, got %v", err)
	}
}

func TestAuthService_InactiveUser(t *testing.T) {
	ctx := context.Background()
	userRepo := newMockUserRepo()
	sessRepo := newMockSessionRepo()
	authSvc := NewAuthService(nil, userRepo, sessRepo)

	pwHash, _ := bcrypt.GenerateFromPassword([]byte("pass123"), bcrypt.DefaultCost)
	user := &domain.User{
		ID:           2,
		Name:         "Pengurus Nonaktif",
		Username:     "nonaktif",
		PasswordHash: string(pwHash),
		IsActive:     false,
		CreatedAt:    time.Now().UTC(),
	}
	_ = userRepo.Create(ctx, nil, user)

	_, _, _, err := authSvc.Login(ctx, "", "nonaktif", "pass123")
	if !errors.Is(err, ErrUserInactive) {
		t.Fatalf("expected ErrUserInactive, got %v", err)
	}
}
