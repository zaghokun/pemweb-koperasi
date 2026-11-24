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
	ErrSavingNotFound         = errors.New("transaksi simpanan tidak ditemukan")
	ErrInsufficientBalance    = errors.New("saldo simpanan sukarela tidak mencukupi untuk penarikan")
	ErrInvalidPeriod          = errors.New("periode simpanan wajib tidak boleh sebelum bulan anggota bergabung")
	ErrRequestKeyConflict     = errors.New("kunci transaksi (request key) telah digunakan dengan data berbeda")
	ErrDuplicateMandatoryPlan = errors.New("simpanan wajib untuk periode tersebut sudah pernah disetor")
)

type DepositMandatoryInput struct {
	MemberID        int64
	UserID          int64
	PeriodMonth     time.Time // Harus tanggal 1 bulan terkait
	TransactionDate time.Time
	RequestKey      string
	Description     string
}

type DepositVoluntaryInput struct {
	MemberID        int64
	UserID          int64
	Amount          int64
	TransactionDate time.Time
	RequestKey      string
	Description     string
}

type WithdrawVoluntaryInput struct {
	MemberID        int64
	UserID          int64
	Amount          int64
	TransactionDate time.Time
	RequestKey      string
	Description     string
}

type SavingService struct {
	db         *sql.DB
	savingRepo *repository.SavingRepository
	memberRepo *repository.MemberRepository
}

func NewSavingService(
	db *sql.DB,
	savingRepo *repository.SavingRepository,
	memberRepo *repository.MemberRepository,
) *SavingService {
	return &SavingService{
		db:         db,
		savingRepo: savingRepo,
		memberRepo: memberRepo,
	}
}

// DepositMandatory mencatat setoran simpanan wajib Rp100.000 untuk satu periode bulan spesifik.
func (s *SavingService) DepositMandatory(ctx context.Context, input DepositMandatoryInput) (*domain.Saving, error) {
	reqKey := strings.TrimSpace(input.RequestKey)
	if reqKey == "" {
		return nil, errors.New("request key diperlukan untuk transaksi simpanan")
	}

	normTxDate, err := domain.NormalizeBusinessDate(input.TransactionDate)
	if err != nil {
		return nil, err
	}

	// Normalisasi period_month ke tanggal 1 bulan tersebut
	normPeriod, err := domain.NormalizeBusinessDate(input.PeriodMonth)
	if err != nil {
		return nil, err
	}
	periodFirstOfMonth := time.Date(normPeriod.Year(), normPeriod.Month(), 1, 0, 0, 0, 0, time.UTC)

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

	existing, err := s.savingRepo.GetByRequestKey(ctx, tx, reqKey)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		if existing.MemberID == input.MemberID && existing.TransactionDate.Equal(normTxDate) &&
			existing.SavingType == domain.SavingMandatory &&
			existing.TransactionType == domain.SavingDeposit &&
			existing.PeriodMonth != nil && existing.PeriodMonth.Equal(periodFirstOfMonth) &&
			existing.Amount == domain.MandatorySavingAmount {
			return existing, nil
		}
		return nil, ErrRequestKeyConflict
	}

	if member.Status != domain.MemberActive {
		return nil, ErrMemberNotActive
	}
	if err := validateMemberDate(ctx, tx, s.memberRepo, member, normTxDate); err != nil {
		return nil, err
	}

	// Periode tidak boleh sebelum bulan anggota bergabung
	memberJoinMonth := time.Date(member.JoinDate.Year(), member.JoinDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	if periodFirstOfMonth.Before(memberJoinMonth) {
		return nil, ErrInvalidPeriod
	}

	now := time.Now().UTC()
	var desc *string
	if strings.TrimSpace(input.Description) != "" {
		d := strings.TrimSpace(input.Description)
		desc = &d
	}
	record := &domain.Saving{
		MemberID:        input.MemberID,
		UserID:          input.UserID,
		SavingType:      domain.SavingMandatory,
		TransactionType: domain.SavingDeposit,
		Amount:          domain.MandatorySavingAmount,
		PeriodMonth:     &periodFirstOfMonth,
		TransactionDate: normTxDate,
		RequestKey:      reqKey,
		Description:     desc,
		CreatedAt:       now,
	}

	if err := s.savingRepo.Create(ctx, tx, record); err != nil {
		// Jika duplikat key period_month (uq_member_mandatory_period)
		if strings.Contains(strings.ToLower(err.Error()), "uq_member_mandatory_period") ||
			strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrDuplicateMandatoryPlan
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return record, nil
}

// DepositVoluntary mencatat setoran simpanan sukarela dengan nominal positif.
func (s *SavingService) DepositVoluntary(ctx context.Context, input DepositVoluntaryInput) (*domain.Saving, error) {
	if input.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	reqKey := strings.TrimSpace(input.RequestKey)
	if reqKey == "" {
		return nil, errors.New("request key diperlukan untuk transaksi simpanan")
	}

	normTxDate, err := domain.NormalizeBusinessDate(input.TransactionDate)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	member, err := s.memberRepo.GetByIDForUpdate(ctx, tx, input.MemberID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}

	existing, err := s.savingRepo.GetByRequestKey(ctx, tx, reqKey)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		if existing.MemberID == input.MemberID && existing.TransactionDate.Equal(normTxDate) &&
			existing.SavingType == domain.SavingVoluntary &&
			existing.TransactionType == domain.SavingDeposit &&
			existing.Amount == input.Amount {
			return existing, nil
		}
		return nil, ErrRequestKeyConflict
	}

	if member.Status != domain.MemberActive {
		return nil, ErrMemberNotActive
	}
	if err := validateMemberDate(ctx, tx, s.memberRepo, member, normTxDate); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var desc *string
	if strings.TrimSpace(input.Description) != "" {
		d := strings.TrimSpace(input.Description)
		desc = &d
	}
	record := &domain.Saving{
		MemberID:        input.MemberID,
		UserID:          input.UserID,
		SavingType:      domain.SavingVoluntary,
		TransactionType: domain.SavingDeposit,
		Amount:          input.Amount,
		PeriodMonth:     nil,
		TransactionDate: normTxDate,
		RequestKey:      reqKey,
		Description:     desc,
		CreatedAt:       now,
	}

	if err := s.savingRepo.Create(ctx, tx, record); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return record, nil
}

// WithdrawVoluntary mencatat penarikan simpanan sukarela dengan row lock dan verifikasi saldo cukup.
func (s *SavingService) WithdrawVoluntary(ctx context.Context, input WithdrawVoluntaryInput) (*domain.Saving, error) {
	if input.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	reqKey := strings.TrimSpace(input.RequestKey)
	if reqKey == "" {
		return nil, errors.New("request key diperlukan untuk transaksi penarikan")
	}

	normTxDate, err := domain.NormalizeBusinessDate(input.TransactionDate)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	member, err := s.memberRepo.GetByIDForUpdate(ctx, tx, input.MemberID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}

	existing, err := s.savingRepo.GetByRequestKey(ctx, tx, reqKey)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		if existing.MemberID == input.MemberID && existing.TransactionDate.Equal(normTxDate) &&
			existing.SavingType == domain.SavingVoluntary &&
			existing.TransactionType == domain.SavingWithdrawal &&
			existing.Amount == input.Amount {
			return existing, nil
		}
		return nil, ErrRequestKeyConflict
	}

	if member.Status != domain.MemberActive {
		return nil, ErrMemberNotActive
	}
	if err := validateMemberDate(ctx, tx, s.memberRepo, member, normTxDate); err != nil {
		return nil, err
	}

	// Hitung saldo sukarela saat ini di bawah row lock
	_, _, sukarela, err := s.savingRepo.GetBalances(ctx, tx, input.MemberID)
	if err != nil {
		return nil, err
	}

	if input.Amount > sukarela {
		return nil, ErrInsufficientBalance
	}

	now := time.Now().UTC()
	var desc *string
	if strings.TrimSpace(input.Description) != "" {
		d := strings.TrimSpace(input.Description)
		desc = &d
	}
	record := &domain.Saving{
		MemberID:        input.MemberID,
		UserID:          input.UserID,
		SavingType:      domain.SavingVoluntary,
		TransactionType: domain.SavingWithdrawal,
		Amount:          input.Amount,
		PeriodMonth:     nil,
		TransactionDate: normTxDate,
		RequestKey:      reqKey,
		Description:     desc,
		CreatedAt:       now,
	}

	if err := s.savingRepo.Create(ctx, tx, record); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return record, nil
}

// GetByID mengambil satu transaksi simpanan beserta data anggotanya untuk bukti kuitansi.
func (s *SavingService) GetByID(ctx context.Context, id int64) (*domain.Saving, *domain.Member, error) {
	saving, err := s.savingRepo.GetByID(ctx, s.db, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrSavingNotFound
		}
		return nil, nil, err
	}

	member, err := s.memberRepo.GetByID(ctx, s.db, saving.MemberID)
	if err != nil {
		return nil, nil, err
	}

	return saving, member, nil
}

// List mengambil daftar riwayat transaksi simpanan dengan pagination dan filter opsional.
func (s *SavingService) List(ctx context.Context, memberID *int64, savingType *domain.SavingType, page, limit int) ([]domain.Saving, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	offset := (page - 1) * limit
	return s.savingRepo.List(ctx, s.db, memberID, savingType, limit, offset)
}
