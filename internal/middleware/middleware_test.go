package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
	"koperasi/internal/service"
)

type dummyUserRepo struct{}

func (d *dummyUserRepo) GetByID(ctx context.Context, db repository.DBTX, id int64) (*domain.User, error) {
	if id == 1 {
		return &domain.User{ID: 1, Name: "Pengurus", Username: "pengurus", IsActive: true}, nil
	}
	return nil, repository.ErrNotFound
}

func (d *dummyUserRepo) GetByUsername(ctx context.Context, db repository.DBTX, username string) (*domain.User, error) {
	return nil, repository.ErrNotFound
}

func (d *dummyUserRepo) Create(ctx context.Context, db repository.DBTX, user *domain.User) error {
	return nil
}

type dummySessionRepo struct {
	sessions map[[32]byte]*domain.Session
}

func (d *dummySessionRepo) Create(ctx context.Context, db repository.DBTX, s *domain.Session) error {
	d.sessions[s.TokenHash] = s
	return nil
}

func (d *dummySessionRepo) GetByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) (*domain.Session, error) {
	s, ok := d.sessions[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return s, nil
}

func (d *dummySessionRepo) DeleteByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) error {
	delete(d.sessions, tokenHash)
	return nil
}

func (d *dummySessionRepo) DeleteExpired(ctx context.Context, db repository.DBTX, now time.Time) error {
	return nil
}

func TestSessionAndAuthMiddleware(t *testing.T) {
	sessRepo := &dummySessionRepo{sessions: make(map[[32]byte]*domain.Session)}
	userRepo := &dummyUserRepo{}
	authSvc := service.NewAuthService(nil, userRepo, sessRepo)

	// Buat sesi aktif untuk user 1
	token := "abcdef1234567890abcdef1234567890"
	tokenHash := service.HashToken(token)
	uid := int64(1)
	sessRepo.sessions[tokenHash] = &domain.Session{
		ID:        10,
		TokenHash: tokenHash,
		UserID:    &uid,
		CSRFToken: "valid_csrf_token",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}

	sessionMw := SessionMiddleware(authSvc, false)
	protectedHandler := sessionMw(RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil {
			t.Fatal("expected user in context")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK:" + u.Name))
	})))

	// 1. Request tanpa cookie -> redirect ke /login
	req1 := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec1 := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec1.Code)
	}
	if rec1.Header().Get("Location") != "/login" {
		t.Errorf("expected redirect to /login, got %s", rec1.Header().Get("Location"))
	}

	// 2. Request dengan valid session cookie -> 200 OK
	req2 := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req2.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rec2 := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "OK:Pengurus") {
		t.Errorf("unexpected body: %s", rec2.Body.String())
	}
}

func TestCSRFMiddleware(t *testing.T) {
	handler := RequireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("MUTATED"))
	}))

	sess := &domain.Session{CSRFToken: "correct_csrf"}
	ctxWithSess := context.WithValue(context.Background(), sessionCtxKey, sess)

	// 1. GET request passes without CSRF
	reqGet := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctxWithSess)
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Errorf("expected 200 on GET, got %d", recGet.Code)
	}

	// 2. POST without CSRF fails with 403
	reqPostNoCSRF := httptest.NewRequest(http.MethodPost, "/test", nil).WithContext(ctxWithSess)
	recPostNoCSRF := httptest.NewRecorder()
	handler.ServeHTTP(recPostNoCSRF, reqPostNoCSRF)
	if recPostNoCSRF.Code != http.StatusForbidden {
		t.Errorf("expected 403 on missing CSRF, got %d", recPostNoCSRF.Code)
	}

	// 3. POST with wrong CSRF fails with 403
	formWrong := url.Values{"_csrf": {"wrong_csrf"}}
	reqPostWrong := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formWrong.Encode())).WithContext(ctxWithSess)
	reqPostWrong.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPostWrong := httptest.NewRecorder()
	handler.ServeHTTP(recPostWrong, reqPostWrong)
	if recPostWrong.Code != http.StatusForbidden {
		t.Errorf("expected 403 on wrong CSRF, got %d", recPostWrong.Code)
	}

	// 4. POST with correct CSRF via form succeeds
	formValid := url.Values{"_csrf": {"correct_csrf"}}
	reqPostValid := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formValid.Encode())).WithContext(ctxWithSess)
	reqPostValid.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPostValid := httptest.NewRecorder()
	handler.ServeHTTP(recPostValid, reqPostValid)
	if recPostValid.Code != http.StatusOK {
		t.Errorf("expected 200 on valid CSRF, got %d", recPostValid.Code)
	}
}
