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
	ErrLoanNotFound         = errors.New("data pinjaman tidak ditemukan")
	ErrExceedsMaxLoanLimit  = errors.New("nominal pinjaman melebihi batas maksimum 3x simpanan pokok dan wajib")
	ErrInvalidLoanStatus    = errors.New("status pinjaman tidak mengizinkan operasi ini")
	ErrRejectionReasonEmpty = errors.New("alasan penolakan pinjaman wajib diisi")
	ErrDisburseBeforeApply  = errors.New("tanggal pencairan tidak boleh mendahului tanggal pengajuan pinjaman")
)

type ApplyLoanInput struct {
	MemberID        int64
	Principal       int64
	TenorMonths     int
	ApplicationDate time.Time
	UserID          int64
}

type LoanDetail struct {
	Loan         domain.Loan
	Member       domain.Member
	Installments []domain.Installment
	TotalPaid    int64
	Remaining    int64
}

type LoanService struct {
	db              *sql.DB
	loanRepo        *repository.LoanRepository
	installmentRepo *repository.InstallmentRepository
	memberRepo      *repository.MemberRepository
	savingRepo      *repository.SavingRepository
}

func NewLoanService(
	db *sql.DB,
	loanRepo *repository.LoanRepository,
	installmentRepo *repository.InstallmentRepository,
	memberRepo *repository.MemberRepository,
	savingRepo *repository.SavingRepository,
) *LoanService {
	return &LoanService{
		db:              db,
		loanRepo:        loanRepo,
		installmentRepo: installmentRepo,
		memberRepo:      memberRepo,
		savingRepo:      savingRepo,
	}
}

// Apply mengajukan pinjaman baru dengan validasi tenor 6-24 bulan, status anggota aktif, dan batas 3x simpanan.
func (s *LoanService) Apply(ctx context.Context, input ApplyLoanInput) (*domain.Loan, error) {
	if input.Principal <= 0 {
		return nil, domain.ErrInvalidAmount
	}
	if input.TenorMonths < domain.MinTenorMonths || input.TenorMonths > domain.MaxTenorMonths {
		return nil, domain.ErrInvalidTenor
	}

	normAppDate, err := domain.NormalizeBusinessDate(input.ApplicationDate)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Kunci anggota FOR UPDATE
	member, err := s.memberRepo.GetByIDForUpdate(ctx, tx, input.MemberID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}

	if member.Status != domain.MemberActive {
		return nil, ErrMemberNotActive
	}

	if err := validateMemberDate(ctx, tx, s.memberRepo, member, normAppDate); err != nil {
		return nil, err
	}

	// Pastikan tidak ada pinjaman terbuka (PENDING, APPROVED, ACTIVE)
	hasOpen, err := s.loanRepo.HasOpenLoan(ctx, tx, input.MemberID)
	if err != nil {
		return nil, err
	}
	if hasOpen {
		return nil, ErrMemberHasOpenLoan
	}

	// Periksa batas maksimum pinjaman = 3 * (Pokok + Wajib)
	pokok, wajib, _, err := s.savingRepo.GetBalances(ctx, tx, input.MemberID)
	if err != nil {
		return nil, err
	}
	baseSavings, err := domain.AddMoney(pokok, wajib)
	if err != nil {
		return nil, err
	}
	maxLimit, err := domain.MultiplyMoney(baseSavings, 3)
	if err != nil {
		return nil, err
	}

	if input.Principal > maxLimit {
		return nil, ErrExceedsMaxLoanLimit
	}

	now := time.Now().UTC()
	loan := &domain.Loan{
		MemberID:        input.MemberID,
		Principal:       input.Principal,
		TenorMonths:     input.TenorMonths,
		ApplicationDate: normAppDate,
		Status:          domain.LoanPending,
		CreatedBy:       input.UserID,
		CreatedAt:       now,
	}

	if err := s.loanRepo.Create(ctx, tx, loan); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return loan, nil
}

// Approve menyetujui pengajuan pinjaman yang berstatus PENDING.
func (s *LoanService) Approve(ctx context.Context, loanID int64, userID int64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	loan, _, err := lockLoanAndMember(ctx, tx, s.loanRepo, s.memberRepo, loanID)
	if err != nil {
		return err
	}

	if loan.Status != domain.LoanPending {
		return ErrInvalidLoanStatus
	}

	if err := s.loanRepo.UpdateStatus(ctx, tx, loanID, domain.LoanApproved, &userID, nil, nil, nil, nil, nil); err != nil {
		return err
	}

	return tx.Commit()
}

// Reject menolak pengajuan pinjaman (dari PENDING atau batalkan APPROVED) dengan mencatat alasan penolakan.
func (s *LoanService) Reject(ctx context.Context, loanID int64, reason string, userID int64) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrRejectionReasonEmpty
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	loan, _, err := lockLoanAndMember(ctx, tx, s.loanRepo, s.memberRepo, loanID)
	if err != nil {
		return err
	}

	if loan.Status != domain.LoanPending && loan.Status != domain.LoanApproved {
		return ErrInvalidLoanStatus
	}

	if err := s.loanRepo.UpdateStatus(ctx, tx, loanID, domain.LoanRejected, loan.ApprovedBy, nil, &userID, nil, nil, &reason); err != nil {
		return err
	}

	return tx.Commit()
}

// Disburse mencairkan pinjaman APPROVED, membuat jadwal angsuran tanggal 10 atomik, dan mengubah status ke ACTIVE.
func (s *LoanService) Disburse(ctx context.Context, loanID int64, disbursementDate time.Time, userID int64) ([]domain.Installment, error) {
	normDisburseDate, err := domain.NormalizeBusinessDate(disbursementDate)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	loan, member, err := lockLoanAndMember(ctx, tx, s.loanRepo, s.memberRepo, loanID)
	if err != nil {
		return nil, err
	}

	if loan.Status != domain.LoanApproved {
		return nil, ErrInvalidLoanStatus
	}

	if member.Status != domain.MemberActive {
		return nil, ErrMemberNotActive
	}

	if normDisburseDate.Before(loan.ApplicationDate) {
		return nil, ErrDisburseBeforeApply
	}

	if err := validateMemberDate(ctx, tx, s.memberRepo, member, normDisburseDate); err != nil {
		return nil, err
	}

	// Validasi ulang batas pinjaman saat pencairan
	pokok, wajib, _, err := s.savingRepo.GetBalances(ctx, tx, loan.MemberID)
	if err != nil {
		return nil, err
	}
	baseSavings, err := domain.AddMoney(pokok, wajib)
	if err != nil {
		return nil, err
	}
	maxLimit, err := domain.MultiplyMoney(baseSavings, 3)
	if err != nil {
		return nil, err
	}
	if loan.Principal > maxLimit {
		return nil, ErrExceedsMaxLoanLimit
	}

	// Bentuk jadwal angsuran menggunakan domain calculator murni (KD-01, BR-08, BR-09)
	schedule, err := domain.GenerateSchedule(loan.Principal, loan.TenorMonths, normDisburseDate)
	if err != nil {
		return nil, err
	}

	// Update status pinjaman menjadi ACTIVE
	if err := s.loanRepo.UpdateStatus(ctx, tx, loanID, domain.LoanActive, loan.ApprovedBy, &userID, nil, &normDisburseDate, nil, nil); err != nil {
		return nil, err
	}

	// Simpan baris jadwal ke tabel installments
	if err := s.installmentRepo.CreateBatch(ctx, tx, loanID, schedule); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return schedule, nil
}

// GetDetail mengambil data lengkap pinjaman, anggota peminjam, dan jadwal angsuran.
func (s *LoanService) GetDetail(ctx context.Context, loanID int64) (*LoanDetail, error) {
	loan, err := s.loanRepo.GetByID(ctx, s.db, loanID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrLoanNotFound
		}
		return nil, err
	}

	member, err := s.memberRepo.GetByID(ctx, s.db, loan.MemberID)
	if err != nil {
		return nil, err
	}

	installments, err := s.installmentRepo.GetByLoanID(ctx, s.db, loanID)
	if err != nil {
		return nil, err
	}

	return &LoanDetail{
		Loan:         *loan,
		Member:       *member,
		Installments: installments,
	}, nil
}

// List mengambil daftar pinjaman dengan pagination dan filter status/anggota.
func (s *LoanService) List(ctx context.Context, memberID *int64, status *domain.LoanStatus, page, limit int) ([]domain.Loan, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	offset := (page - 1) * limit
	return s.loanRepo.List(ctx, s.db, memberID, status, limit, offset)
}
