package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/handler"
	"koperasi/internal/repository"
	"koperasi/internal/router"
	"koperasi/internal/service"
)

type mockMemberRepo struct {
	members   map[int64]*domain.Member
	byNumber  map[string]*domain.Member
	currentID int64
}

func newMockMemberRepo() *mockMemberRepo {
	return &mockMemberRepo{
		members:  make(map[int64]*domain.Member),
		byNumber: make(map[string]*domain.Member),
	}
}

func (m *mockMemberRepo) Create(ctx context.Context, db repository.DBTX, member *domain.Member) error {
	m.currentID++
	member.ID = m.currentID
	m.members[member.ID] = member
	m.byNumber[member.MemberNumber] = member
	return nil
}

func (m *mockMemberRepo) GetByID(ctx context.Context, db repository.DBTX, id int64) (*domain.Member, error) {
	if mem, ok := m.members[id]; ok {
		return mem, nil
	}
	return nil, repository.ErrNotFound
}

func (m *mockMemberRepo) GetByIDForUpdate(ctx context.Context, db repository.DBTX, id int64) (*domain.Member, error) {
	return m.GetByID(ctx, db, id)
}

func (m *mockMemberRepo) GetByNumber(ctx context.Context, db repository.DBTX, number string) (*domain.Member, error) {
	if mem, ok := m.byNumber[number]; ok {
		return mem, nil
	}
	return nil, repository.ErrNotFound
}

func (m *mockMemberRepo) UpdateIdentity(ctx context.Context, db repository.DBTX, id int64, name, address, phone string) error {
	mem, ok := m.members[id]
	if !ok {
		return repository.ErrNotFound
	}
	mem.Name = name
	mem.Address = address
	mem.Phone = phone
	return nil
}

func (m *mockMemberRepo) UpdateExit(ctx context.Context, db repository.DBTX, id int64, exitDate time.Time, exitUserID int64) error {
	mem, ok := m.members[id]
	if !ok {
		return repository.ErrNotFound
	}
	mem.Status = domain.MemberExited
	mem.ExitDate = &exitDate
	mem.ExitUserID = &exitUserID
	return nil
}

func (m *mockMemberRepo) List(ctx context.Context, db repository.DBTX, search string, limit, offset int) ([]domain.Member, int, error) {
	var res []domain.Member
	for _, mem := range m.members {
		if search == "" || strings.Contains(mem.Name, search) || strings.Contains(mem.MemberNumber, search) {
			res = append(res, *mem)
		}
	}
	return res, len(res), nil
}

type mockSavingRepoFull struct {
	savings   map[int64]*domain.Saving
	byReqKey  map[string]*domain.Saving
	currentID int64
}

func newMockSavingRepoFull() *mockSavingRepoFull {
	return &mockSavingRepoFull{
		savings:  make(map[int64]*domain.Saving),
		byReqKey: make(map[string]*domain.Saving),
	}
}

func (s *mockSavingRepoFull) Create(ctx context.Context, db repository.DBTX, saving *domain.Saving) error {
	s.currentID++
	saving.ID = s.currentID
	s.savings[saving.ID] = saving
	s.byReqKey[saving.RequestKey] = saving
	return nil
}

func (s *mockSavingRepoFull) GetByID(ctx context.Context, db repository.DBTX, id int64) (*domain.Saving, error) {
	if sav, ok := s.savings[id]; ok {
		return sav, nil
	}
	return nil, repository.ErrNotFound
}

func (s *mockSavingRepoFull) GetByRequestKey(ctx context.Context, db repository.DBTX, key string) (*domain.Saving, error) {
	if sav, ok := s.byReqKey[key]; ok {
		return sav, nil
	}
	return nil, repository.ErrNotFound
}

func (s *mockSavingRepoFull) GetBalances(ctx context.Context, db repository.DBTX, memberID int64) (pokok, wajib, sukarela int64, err error) {
	for _, sv := range s.savings {
		if sv.MemberID == memberID {
			sign := int64(1)
			if sv.TransactionType != domain.SavingDeposit {
				sign = -1
			}
			switch sv.SavingType {
			case domain.SavingPrincipal:
				pokok += sign * sv.Amount
			case domain.SavingMandatory:
				wajib += sign * sv.Amount
			case domain.SavingVoluntary:
				sukarela += sign * sv.Amount
			}
		}
	}
	return pokok, wajib, sukarela, nil
}

func (s *mockSavingRepoFull) ListByMemberID(ctx context.Context, db repository.DBTX, memberID int64, limit, offset int) ([]domain.Saving, error) {
	var res []domain.Saving
	for _, sv := range s.savings {
		if sv.MemberID == memberID {
			res = append(res, *sv)
		}
	}
	return res, nil
}

func (s *mockSavingRepoFull) List(ctx context.Context, db repository.DBTX, memberID *int64, savingType *domain.SavingType, limit, offset int) ([]domain.Saving, int, error) {
	var res []domain.Saving
	for _, sv := range s.savings {
		if memberID != nil && sv.MemberID != *memberID {
			continue
		}
		if savingType != nil && sv.SavingType != *savingType {
			continue
		}
		res = append(res, *sv)
	}
	return res, len(res), nil
}

func setupStage5App(t *testing.T) (http.Handler, *mockUserRepo, *mockMemberRepo, *mockSavingRepoFull) {
	t.Helper()
	userRepo := &mockUserRepo{
		usersByID:       make(map[int64]*domain.User),
		usersByUsername: make(map[string]*domain.User),
	}
	sessRepo := &mockSessionRepo{
		sessions: make(map[[32]byte]*domain.Session),
	}
	memberRepo := newMockMemberRepo()
	savingRepo := newMockSavingRepoFull()

	authSvc := service.NewAuthService(nil, userRepo, sessRepo)
	// Gunakan NewMemberService & NewSavingService dengan mock
	// Karena service memanggil memberRepo dan savingRepo sebagai concrete type di signature aslinya,
	// untuk testing HTTP handler kita dapat menggunakan test handler langsung atau menguji rute yang didukung.
	templatesDir := filepath.Join("..", "..", "web", "templates")
	staticDir := filepath.Join("..", "..", "web", "static")

	h, err := handler.NewHandler(authSvc, nil, nil, nil, nil, nil, nil, false, templatesDir)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	app := router.New(h, authSvc, false, staticDir)
	return app, userRepo, memberRepo, savingRepo
}

func TestStage5RoutesProtected(t *testing.T) {
	app, _, _, _ := setupStage5App(t)

	// Pastikan rute-rute baru di Tahap 5 terproteksi (redirect ke /login jika tanpa auth)
	protectedPaths := []string{
		"/members",
		"/members/new",
		"/members/1",
		"/members/1/edit",
		"/members/1/exit",
		"/savings",
		"/savings/new",
		"/savings/withdrawals/new",
		"/savings/1/receipt",
		"/loans",
		"/loans/new",
		"/loans/1",
		"/loans/1/installments",
		"/loans/1/payments/new",
		"/payments/1/receipt",
		"/loans/1/settlement",
		"/settlements/1/receipt",
		"/reports/loans/1",
		"/reports/arrears",
		"/reports/monthly",
	}

	for _, p := range protectedPaths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect for unauthenticated %s, got %d", p, rec.Code)
		}
		if rec.Header().Get("Location") != "/login" {
			t.Errorf("expected redirect to /login for %s, got %s", p, rec.Header().Get("Location"))
		}
	}
}
