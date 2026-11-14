package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"koperasi/internal/domain"
	"koperasi/internal/handler"
	"koperasi/internal/middleware"
	"koperasi/internal/repository"
	"koperasi/internal/router"
	"koperasi/internal/service"
)

type mockUserRepo struct {
	usersByID       map[int64]*domain.User
	usersByUsername map[string]*domain.User
}

func (m *mockUserRepo) GetByID(ctx context.Context, db repository.DBTX, id int64) (*domain.User, error) {
	if u, ok := m.usersByID[id]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}

func (m *mockUserRepo) GetByUsername(ctx context.Context, db repository.DBTX, username string) (*domain.User, error) {
	if u, ok := m.usersByUsername[strings.ToLower(username)]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}

func (m *mockUserRepo) Create(ctx context.Context, db repository.DBTX, user *domain.User) error {
	m.usersByID[user.ID] = user
	m.usersByUsername[strings.ToLower(user.Username)] = user
	return nil
}

type mockSessionRepo struct {
	sessions map[[32]byte]*domain.Session
}

func (m *mockSessionRepo) Create(ctx context.Context, db repository.DBTX, s *domain.Session) error {
	m.sessions[s.TokenHash] = s
	return nil
}

func (m *mockSessionRepo) GetByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) (*domain.Session, error) {
	if s, ok := m.sessions[tokenHash]; ok {
		return s, nil
	}
	return nil, repository.ErrNotFound
}

func (m *mockSessionRepo) DeleteByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) error {
	delete(m.sessions, tokenHash)
	return nil
}

func (m *mockSessionRepo) DeleteExpired(ctx context.Context, db repository.DBTX, now time.Time) error {
	return nil
}

func setupTestApp(t *testing.T) (http.Handler, *mockUserRepo, *mockSessionRepo) {
	t.Helper()
	userRepo := &mockUserRepo{
		usersByID:       make(map[int64]*domain.User),
		usersByUsername: make(map[string]*domain.User),
	}
	sessRepo := &mockSessionRepo{
		sessions: make(map[[32]byte]*domain.Session),
	}
	authSvc := service.NewAuthService(nil, userRepo, sessRepo)

	// Path template test
	templatesDir := filepath.Join("..", "..", "web", "templates")
	staticDir := filepath.Join("..", "..", "web", "static")

	memberRepo := repository.NewMemberRepository()
	savingRepo := repository.NewSavingRepository()
	loanRepo := repository.NewLoanRepository()
	installmentRepo := repository.NewInstallmentRepository()
	memberSvc := service.NewMemberService(nil, memberRepo, savingRepo, loanRepo)
	savingSvc := service.NewSavingService(nil, savingRepo, memberRepo)
	loanSvc := service.NewLoanService(nil, loanRepo, installmentRepo, memberRepo, savingRepo)

	h, err := handler.NewHandler(authSvc, memberSvc, savingSvc, loanSvc, false, templatesDir)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	app := router.New(h, authSvc, false, staticDir)
	return app, userRepo, sessRepo
}

func extractCSRFFromHTML(html string) string {
	marker := `name="_csrf" value="`
	idx := strings.Index(html, marker)
	if idx == -1 {
		return ""
	}
	start := idx + len(marker)
	end := strings.Index(html[start:], `"`)
	if end == -1 {
		return ""
	}
	return html[start : start+end]
}

func TestAuthHTTPFlow(t *testing.T) {
	app, userRepo, _ := setupTestApp(t)

	// Seed satu pengurus
	pwHash, _ := bcrypt.GenerateFromPassword([]byte("passwordku123"), bcrypt.DefaultCost)
	_ = userRepo.Create(context.Background(), nil, &domain.User{
		ID:           1,
		Name:         "Budi Pengurus",
		Username:     "budi",
		PasswordHash: string(pwHash),
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	})

	// 1. GET /login -> Form login tampil dan cookie sesi pra-login disetel
	reqGetLogin := httptest.NewRequest(http.MethodGet, "/login", nil)
	recGetLogin := httptest.NewRecorder()
	app.ServeHTTP(recGetLogin, reqGetLogin)

	if recGetLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET /login, got %d", recGetLogin.Code)
	}

	preCookies := recGetLogin.Result().Cookies()
	var preCookie *http.Cookie
	for _, c := range preCookies {
		if c.Name == middleware.SessionCookieName {
			preCookie = c
			break
		}
	}
	if preCookie == nil || preCookie.Value == "" {
		t.Fatal("expected pre-login session cookie to be set")
	}

	csrfToken := extractCSRFFromHTML(recGetLogin.Body.String())
	if csrfToken == "" {
		t.Fatal("expected CSRF token to be rendered in HTML form")
	}

	// 2. POST /login dengan password salah -> Login gagal, pesan error tampil
	badForm := url.Values{
		"_csrf":    {csrfToken},
		"username": {"budi"},
		"password": {"salah"},
	}
	reqBadLogin := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(badForm.Encode()))
	reqBadLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqBadLogin.AddCookie(preCookie)
	recBadLogin := httptest.NewRecorder()
	app.ServeHTTP(recBadLogin, reqBadLogin)

	if recBadLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with error form on bad credentials, got %d", recBadLogin.Code)
	}
	if !strings.Contains(recBadLogin.Body.String(), "Username atau password salah") {
		t.Errorf("expected error message in body, got: %s", recBadLogin.Body.String())
	}

	// Ambil CSRF baru dari form login yang di-refresh
	newCSRF := extractCSRFFromHTML(recBadLogin.Body.String())
	freshCookies := recBadLogin.Result().Cookies()
	var freshCookie *http.Cookie
	for _, c := range freshCookies {
		if c.Name == middleware.SessionCookieName {
			freshCookie = c
			break
		}
	}
	if freshCookie != nil {
		preCookie = freshCookie
	}

	// 3. POST /login tanpa CSRF -> 403 Forbidden
	reqNoCSRF := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{
		"username": {"budi"},
		"password": {"passwordku123"},
	}.Encode()))
	reqNoCSRF.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqNoCSRF.AddCookie(preCookie)
	recNoCSRF := httptest.NewRecorder()
	app.ServeHTTP(recNoCSRF, reqNoCSRF)

	if recNoCSRF.Code != http.StatusForbidden {
		t.Errorf("expected 403 on missing CSRF, got %d", recNoCSRF.Code)
	}

	// 4. POST /login dengan kredensial benar dan CSRF valid -> 303 Redirect ke /dashboard
	validForm := url.Values{
		"_csrf":    {newCSRF},
		"username": {"budi"},
		"password": {"passwordku123"},
	}
	reqValidLogin := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(validForm.Encode()))
	reqValidLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqValidLogin.AddCookie(preCookie)
	recValidLogin := httptest.NewRecorder()
	app.ServeHTTP(recValidLogin, reqValidLogin)

	if recValidLogin.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on successful login, got %d", recValidLogin.Code)
	}
	if recValidLogin.Header().Get("Location") != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %s", recValidLogin.Header().Get("Location"))
	}

	// Ambil cookie sesi terotasi
	var authCookie *http.Cookie
	for _, c := range recValidLogin.Result().Cookies() {
		if c.Name == middleware.SessionCookieName && c.Value != "" {
			authCookie = c
			break
		}
	}
	if authCookie == nil {
		t.Fatal("expected authenticated session cookie to be set")
	}

	// 5. GET /dashboard dengan sesi terautentikasi -> 200 OK
	reqDashboard := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	reqDashboard.AddCookie(authCookie)
	recDashboard := httptest.NewRecorder()
	app.ServeHTTP(recDashboard, reqDashboard)

	if recDashboard.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /dashboard, got %d", recDashboard.Code)
	}
	if !strings.Contains(recDashboard.Body.String(), "Budi Pengurus") {
		t.Errorf("expected logged in user name on dashboard, body: %s", recDashboard.Body.String())
	}

	// Ambil CSRF dari halaman dashboard untuk logout
	dashCSRF := extractCSRFFromHTML(recDashboard.Body.String())

	// 6. POST /logout dengan CSRF valid -> Cookie dihapus dan redirect ke /login
	logoutForm := url.Values{"_csrf": {dashCSRF}}
	reqLogout := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(logoutForm.Encode()))
	reqLogout.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogout.AddCookie(authCookie)
	recLogout := httptest.NewRecorder()
	app.ServeHTTP(recLogout, reqLogout)

	if recLogout.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on logout, got %d", recLogout.Code)
	}
	if recLogout.Header().Get("Location") != "/login" {
		t.Errorf("expected redirect to /login, got %s", recLogout.Header().Get("Location"))
	}

	// 7. GET /dashboard setelah logout -> Redirect ke /login
	reqAfterLogout := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	reqAfterLogout.AddCookie(authCookie)
	recAfterLogout := httptest.NewRecorder()
	app.ServeHTTP(recAfterLogout, reqAfterLogout)

	if recAfterLogout.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect on access after logout, got %d", recAfterLogout.Code)
	}
	if recAfterLogout.Header().Get("Location") != "/login" {
		t.Errorf("expected redirect to /login, got %s", recAfterLogout.Header().Get("Location"))
	}
}
