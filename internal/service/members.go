package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
)

var (
	ErrMemberNotFound      = errors.New("data anggota tidak ditemukan")
	ErrMemberNumberExists  = errors.New("nomor anggota sudah terdaftar")
	ErrMemberNotActive     = errors.New("anggota tidak berstatus aktif")
	ErrMemberHasOpenLoan   = errors.New("anggota masih memiliki pinjaman aktif atau pengajuan terbuka")
	ErrInvalidMemberData   = errors.New("data anggota tidak lengkap atau tidak valid")
	ErrExitDateBeforeJoin  = errors.New("tanggal keluar tidak boleh mendahului tanggal masuk")
)

type MemberDetail struct {
	Member           domain.Member
	PokokBalance     int64
	WajibBalance     int64
	SukarelaBalance  int64
	TotalSavings     int64
	MaxLoanLimit     int64
}

type MemberSummary struct {
	Member       domain.Member
	TotalSavings int64
}

type RegisterMemberInput struct {
	MemberNumber string
	Name         string
	Address      string
	Phone        string
	JoinDate     time.Time
	UserID       int64
}

type ExitResult struct {
	MemberID         int64
	ExitDate         time.Time
	RefundedPokok    int64
	RefundedWajib    int64
	RefundedSukarela int64
	TotalRefunded    int64
}

type MemberService struct {
	db         *sql.DB
	memberRepo *repository.MemberRepository
	savingRepo *repository.SavingRepository
	loanRepo   *repository.LoanRepository
}

func NewMemberService(
	db *sql.DB,
	memberRepo *repository.MemberRepository,
	savingRepo *repository.SavingRepository,
	loanRepo *repository.LoanRepository,
) *MemberService {
	return &MemberService{
		db:         db,
		memberRepo: memberRepo,
		savingRepo: savingRepo,
		loanRepo:   loanRepo,
	}
}

// Register mendaftarkan anggota baru dan mencatat setoran simpanan pokok Rp500.000 secara atomik.
func (s *MemberService) Register(ctx context.Context, input RegisterMemberInput) (*domain.Member, *domain.Saving, error) {
	if strings.TrimSpace(input.MemberNumber) == "" || strings.TrimSpace(input.Name) == "" ||
		strings.TrimSpace(input.Address) == "" || strings.TrimSpace(input.Phone) == "" {
		return nil, nil, ErrInvalidMemberData
	}

	joinDate, err := domain.NormalizeBusinessDate(input.JoinDate)
	if err != nil {
		return nil, nil, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	// Periksa apakah nomor anggota sudah terdaftar
	existing, err := s.memberRepo.GetByNumber(ctx, tx, input.MemberNumber)
	if err == nil && existing != nil {
		return nil, nil, ErrMemberNumberExists
	}

	now := time.Now().UTC()
	member := &domain.Member{
		MemberNumber: input.MemberNumber,
		Name:         input.Name,
		Address:      input.Address,
		Phone:        input.Phone,
		JoinDate:     joinDate,
		Status:       domain.MemberActive,
		CreatedBy:    input.UserID,
		CreatedAt:    now,
	}

	if err := s.memberRepo.Create(ctx, tx, member); err != nil {
		return nil, nil, err
	}

	// Buat setoran pokok awal Rp500.000 atomik
	reqKey, err := generateRandomHex(16)
	if err != nil {
		return nil, nil, err
	}

	desc := "Setoran simpanan pokok pendaftaran"
	saving := &domain.Saving{
		MemberID:        member.ID,
		UserID:          input.UserID,
		SavingType:      domain.SavingPrincipal,
		TransactionType: domain.SavingDeposit,
		Amount:          domain.PrincipalSavingAmount,
		PeriodMonth:     nil,
		TransactionDate: joinDate,
		RequestKey:      reqKey,
		Description:     &desc,
		CreatedAt:       now,
	}

	if err := s.savingRepo.Create(ctx, tx, saving); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return member, saving, nil
}

// GetDetail mengambil detail data anggota beserta saldo agregat dan batas pinjaman 3x (Pokok + Wajib).
func (s *MemberService) GetDetail(ctx context.Context, id int64) (*MemberDetail, error) {
	m, err := s.memberRepo.GetByID(ctx, s.db, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}

	pokok, wajib, sukarela, err := s.savingRepo.GetBalances(ctx, s.db, id)
	if err != nil {
		return nil, err
	}

	totalSavings, err := domain.AddMoney(pokok, wajib)
	if err != nil {
		return nil, err
	}
	totalSavings, err = domain.AddMoney(totalSavings, sukarela)
	if err != nil {
		return nil, err
	}

	baseLimit, err := domain.AddMoney(pokok, wajib)
	if err != nil {
		return nil, err
	}
	maxLoan, err := domain.MultiplyMoney(baseLimit, 3)
	if err != nil {
		return nil, err
	}

	return &MemberDetail{
		Member:          *m,
		PokokBalance:    pokok,
		WajibBalance:    wajib,
		SukarelaBalance: sukarela,
		TotalSavings:    totalSavings,
		MaxLoanLimit:    maxLoan,
	}, nil
}

// UpdateIdentity memperbarui identitas anggota (nama, alamat, telepon).
func (s *MemberService) UpdateIdentity(ctx context.Context, id int64, name, address, phone string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(address) == "" || strings.TrimSpace(phone) == "" {
		return ErrInvalidMemberData
	}
	err := s.memberRepo.UpdateIdentity(ctx, s.db, id, name, address, phone)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrMemberNotFound
	}
	return err
}

// List mengambil daftar anggota dengan pencarian dan pagination.
func (s *MemberService) List(ctx context.Context, search string, page, limit int) ([]MemberSummary, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	offset := (page - 1) * limit

	members, total, err := s.memberRepo.List(ctx, s.db, search, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	var summaries []MemberSummary
	for _, m := range members {
		pokok, wajib, sukarela, err := s.savingRepo.GetBalances(ctx, s.db, m.ID)
		if err != nil {
			return nil, 0, err
		}
		tot, _ := domain.AddMoney(pokok, wajib)
		tot, _ = domain.AddMoney(tot, sukarela)
		summaries = append(summaries, MemberSummary{
			Member:       m,
			TotalSavings: tot,
		})
	}

	return summaries, total, nil
}

// Exit memproses pengeluaran anggota secara atomik sesuai aturan KD-10:
// 1. Kunci anggota FOR UPDATE.
// 2. Pastikan status ACTIVE.
// 3. Pastikan tidak ada pinjaman ACTIVE atau pengajuan PENDING/APPROVED.
// 4. Catat PENGEMBALIAN_KELUAR untuk seluruh saldo simpanan yang tersisa.
// 5. Ubah status menjadi EXITED.
func (s *MemberService) Exit(ctx context.Context, memberID int64, exitDate time.Time, currentUserID int64) (*ExitResult, error) {
	normExitDate, err := domain.NormalizeBusinessDate(exitDate)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	member, err := s.memberRepo.GetByIDForUpdate(ctx, tx, memberID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}

	if member.Status != domain.MemberActive {
		return nil, ErrMemberNotActive
	}

	if normExitDate.Before(member.JoinDate) {
		return nil, ErrExitDateBeforeJoin
	}

	// Cek pinjaman terbuka
	hasOpen, err := s.loanRepo.HasOpenLoan(ctx, tx, memberID)
	if err != nil {
		return nil, err
	}
	if hasOpen {
		return nil, ErrMemberHasOpenLoan
	}

	pokok, wajib, sukarela, err := s.savingRepo.GetBalances(ctx, tx, memberID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var totalRefunded int64

	// Refund pokok jika bersaldo
	if pokok > 0 {
		reqKey, _ := generateRandomHex(16)
		desc := "Pengembalian simpanan pokok anggota keluar"
		sRecord := &domain.Saving{
			MemberID:        memberID,
			UserID:          currentUserID,
			SavingType:      domain.SavingPrincipal,
			TransactionType: domain.SavingExitRefund,
			Amount:          pokok,
			PeriodMonth:     nil,
			TransactionDate: normExitDate,
			RequestKey:      reqKey,
			Description:     &desc,
			CreatedAt:       now,
		}
		if err := s.savingRepo.Create(ctx, tx, sRecord); err != nil {
			return nil, err
		}
		totalRefunded, _ = domain.AddMoney(totalRefunded, pokok)
	}

	// Refund wajib jika bersaldo
	if wajib > 0 {
		reqKey, _ := generateRandomHex(16)
		desc := "Pengembalian simpanan wajib anggota keluar"
		sRecord := &domain.Saving{
			MemberID:        memberID,
			UserID:          currentUserID,
			SavingType:      domain.SavingMandatory,
			TransactionType: domain.SavingExitRefund,
			Amount:          wajib,
			PeriodMonth:     nil,
			TransactionDate: normExitDate,
			RequestKey:      reqKey,
			Description:     &desc,
			CreatedAt:       now,
		}
		if err := s.savingRepo.Create(ctx, tx, sRecord); err != nil {
			return nil, err
		}
		totalRefunded, _ = domain.AddMoney(totalRefunded, wajib)
	}

	// Refund sukarela jika bersaldo
	if sukarela > 0 {
		reqKey, _ := generateRandomHex(16)
		desc := "Pengembalian simpanan sukarela anggota keluar"
		sRecord := &domain.Saving{
			MemberID:        memberID,
			UserID:          currentUserID,
			SavingType:      domain.SavingVoluntary,
			TransactionType: domain.SavingExitRefund,
			Amount:          sukarela,
			PeriodMonth:     nil,
			TransactionDate: normExitDate,
			RequestKey:      reqKey,
			Description:     &desc,
			CreatedAt:       now,
		}
		if err := s.savingRepo.Create(ctx, tx, sRecord); err != nil {
			return nil, err
		}
		totalRefunded, _ = domain.AddMoney(totalRefunded, sukarela)
	}

	// Perbarui status anggota menjadi EXITED
	if err := s.memberRepo.UpdateExit(ctx, tx, memberID, normExitDate, currentUserID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ExitResult{
		MemberID:         memberID,
		ExitDate:         normExitDate,
		RefundedPokok:    pokok,
		RefundedWajib:    wajib,
		RefundedSukarela: sukarela,
		TotalRefunded:    totalRefunded,
	}, nil
}
