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
	ErrSettlementAmountMismatch = errors.New("nominal konfirmasi tidak cocok dengan perhitungan pelunasan terkini")
	ErrSettlementBeforeDisb     = errors.New("tanggal pelunasan tidak boleh mendahului tanggal pencairan")
	ErrLoanNotActiveForSettle   = errors.New("hanya pinjaman berstatus ACTIVE yang dapat dilunasi dipercepat")
	ErrMissingRequestKey        = errors.New("kunci permintaan (request key) wajib diisi")
)

type SettleInput struct {
	LoanID          int64
	UserID          int64
	SettlementDate  time.Time
	ConfirmedAmount int64
	RequestKey      string
}

type SettlementPreviewResult struct {
	Loan            domain.Loan
	Member          domain.Member
	SettlementDate  time.Time
	Breakdown       domain.EarlySettlement
	Balances        []domain.InstallmentBalance
}

type SettleResult struct {
	Payment    domain.Payment
	Settlement domain.LoanSettlement
}

type SettlementReceiptResult struct {
	Payment     domain.Payment
	Loan        domain.Loan
	Member      domain.Member
	Settlement  domain.LoanSettlement
	Allocations []domain.PaymentAllocation
}

type SettlementService struct {
	db              *sql.DB
	settlementRepo  *repository.SettlementRepository
	loanRepo        *repository.LoanRepository
	installmentRepo *repository.InstallmentRepository
	paymentRepo     *repository.PaymentRepository
	memberRepo      *repository.MemberRepository
}

func NewSettlementService(
	db *sql.DB,
	settlementRepo *repository.SettlementRepository,
	loanRepo *repository.LoanRepository,
	installmentRepo *repository.InstallmentRepository,
	paymentRepo *repository.PaymentRepository,
	memberRepo *repository.MemberRepository,
) *SettlementService {
	return &SettlementService{
		db:              db,
		settlementRepo:  settlementRepo,
		loanRepo:        loanRepo,
		installmentRepo: installmentRepo,
		paymentRepo:     paymentRepo,
		memberRepo:      memberRepo,
	}
}

// Preview menghitung pratinjau pelunasan dipercepat tanpa mengubah database (KD-07, KD-08, KD-09).
func (s *SettlementService) Preview(ctx context.Context, loanID int64, settlementDate time.Time) (*SettlementPreviewResult, error) {
	normDate, err := domain.NormalizeBusinessDate(settlementDate)
	if err != nil {
		return nil, err
	}

	loan, err := s.loanRepo.GetByID(ctx, s.db, loanID)
	if err != nil {
		return nil, err
	}
	if loan.Status != domain.LoanActive {
		return nil, ErrLoanNotActiveForSettle
	}
	if loan.DisbursementDate == nil || normDate.Before(*loan.DisbursementDate) {
		return nil, ErrSettlementBeforeDisb
	}

	member, err := s.memberRepo.GetByID(ctx, s.db, loan.MemberID)
	if err != nil {
		return nil, err
	}

	balances, err := s.installmentRepo.GetInstallmentBalances(ctx, s.db, loanID, normDate)
	if err != nil {
		return nil, err
	}

	breakdown, err := domain.PreviewEarlySettlement(*loan.DisbursementDate, balances, normDate)
	if err != nil {
		return nil, err
	}

	return &SettlementPreviewResult{
		Loan:           *loan,
		Member:         *member,
		SettlementDate: normDate,
		Breakdown:      breakdown,
		Balances:       balances,
	}, nil
}

// Settle mengeksekusi pelunasan dipercepat secara atomik di bawah row lock (KD-08, KD-09).
func (s *SettlementService) Settle(ctx context.Context, input SettleInput) (*SettleResult, error) {
	normDate, err := domain.NormalizeBusinessDate(input.SettlementDate)
	if err != nil {
		return nil, err
	}

	reqKey := strings.TrimSpace(input.RequestKey)
	if reqKey == "" {
		return nil, ErrMissingRequestKey
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Kunci pinjaman terlebih dahulu
	loan, err := s.loanRepo.GetByIDForUpdate(ctx, tx, input.LoanID)
	if err != nil {
		return nil, err
	}
	if loan.Status != domain.LoanActive {
		return nil, ErrLoanNotActiveForSettle
	}
	if loan.DisbursementDate == nil || normDate.Before(*loan.DisbursementDate) {
		return nil, ErrSettlementBeforeDisb
	}

	// Kunci anggota
	_, err = s.memberRepo.GetByIDForUpdate(ctx, tx, loan.MemberID)
	if err != nil {
		return nil, err
	}

	// Idempotency check: jika request_key sudah pernah diproses
	existingPayment, err := s.paymentRepo.GetByRequestKey(ctx, tx, reqKey)
	if err == nil && existingPayment != nil {
		if existingPayment.LoanID != input.LoanID {
			return nil, ErrRequestKeyConflict
		}
		existingSettlement, err := s.settlementRepo.GetByPaymentID(ctx, tx, existingPayment.ID)
		if err == nil && existingSettlement != nil {
			_ = tx.Commit()
			return &SettleResult{
				Payment:    *existingPayment,
				Settlement: *existingSettlement,
			}, nil
		}
	}

	// Hitung ulang breakdown pelunasan di bawah lock
	balances, err := s.installmentRepo.GetInstallmentBalances(ctx, tx, input.LoanID, normDate)
	if err != nil {
		return nil, err
	}

	breakdown, err := domain.PreviewEarlySettlement(*loan.DisbursementDate, balances, normDate)
	if err != nil {
		return nil, err
	}

	// Validasi kesesuaian nominal yang dikonfirmasi pengurus
	if input.ConfirmedAmount != breakdown.TotalReceived {
		return nil, ErrSettlementAmountMismatch
	}

	now := time.Now().UTC()

	// 1. Catat payment header dengan jenis EARLY_SETTLEMENT
	payment := &domain.Payment{
		LoanID:      input.LoanID,
		UserID:      input.UserID,
		PaymentType: domain.PaymentEarlySettlement,
		PaymentDate: normDate,
		Amount:      breakdown.TotalReceived,
		RequestKey:  reqKey,
		CreatedAt:   now,
	}
	if err := s.paymentRepo.Create(ctx, tx, payment); err != nil {
		return nil, err
	}

	// Map installment number ke installment ID
	instMap := make(map[int]int64, len(balances))
	for _, b := range balances {
		instMap[b.Installment.InstallmentNumber] = b.Installment.ID
	}

	// 2. Simpan seluruh payment_allocations (denda, jasa, pokok, admin)
	allocations := make([]domain.PaymentAllocation, 0, len(breakdown.Allocations))
	for _, a := range breakdown.Allocations {
		var instID *int64
		if a.Component != domain.ComponentAdministration && a.InstallmentNumber > 0 {
			if id, ok := instMap[a.InstallmentNumber]; ok {
				idVal := id
				instID = &idVal
			}
		}
		allocations = append(allocations, domain.PaymentAllocation{
			PaymentID:     payment.ID,
			InstallmentID: instID,
			ComponentType: a.Component,
			Amount:        a.Amount,
		})
	}
	if err := s.paymentRepo.CreateAllocations(ctx, tx, payment.ID, allocations); err != nil {
		return nil, err
	}

	// 3. Catat pembebasan jasa mendatang (waived interest)
	for _, w := range breakdown.Waivers {
		if id, ok := instMap[w.InstallmentNumber]; ok {
			if err := s.installmentRepo.WaiveInterest(ctx, tx, id, w.Amount, normDate); err != nil {
				return nil, err
			}
		}
	}

	// 4. Tutup seluruh angsuran pada tanggal pelunasan
	for _, b := range balances {
		if err := s.installmentRepo.CloseInstallment(ctx, tx, b.Installment.ID, normDate); err != nil {
			return nil, err
		}
	}

	// 5. Catat snapshot loan_settlements
	currentInstID := instMap[breakdown.CurrentInstallmentNumber]
	settlementRecord := &domain.LoanSettlement{
		LoanID:               input.LoanID,
		PaymentID:            payment.ID,
		CurrentInstallmentID: currentInstID,
		ArrearsAmount:        breakdown.ArrearsAmount,
		RemainingPrincipal:   breakdown.RemainingPrincipal,
		CurrentInterest:      breakdown.CurrentInterest,
		AdministrationFee:    breakdown.AdministrationFee,
		WaivedInterest:       breakdown.WaivedInterest,
	}
	if err := s.settlementRepo.Create(ctx, tx, settlementRecord); err != nil {
		return nil, err
	}

	// 6. Ubah status pinjaman menjadi SETTLED
	if err := s.loanRepo.UpdateStatus(ctx, tx, input.LoanID, domain.LoanSettled, nil, nil, nil, nil, &normDate, nil); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &SettleResult{
		Payment:    *payment,
		Settlement: *settlementRecord,
	}, nil
}

// GetReceipt mengambil data lengkap kuitansi pelunasan dipercepat untuk ditampilkan/dicetak.
func (s *SettlementService) GetReceipt(ctx context.Context, paymentID int64) (*SettlementReceiptResult, error) {
	payment, err := s.paymentRepo.GetByID(ctx, s.db, paymentID)
	if err != nil {
		return nil, err
	}
	if payment.PaymentType != domain.PaymentEarlySettlement {
		return nil, errors.New("bukan transaksi pelunasan dipercepat")
	}

	loan, err := s.loanRepo.GetByID(ctx, s.db, payment.LoanID)
	if err != nil {
		return nil, err
	}

	member, err := s.memberRepo.GetByID(ctx, s.db, loan.MemberID)
	if err != nil {
		return nil, err
	}

	settlement, err := s.settlementRepo.GetByPaymentID(ctx, s.db, paymentID)
	if err != nil {
		return nil, err
	}

	allocations, err := s.paymentRepo.GetAllocationsByPaymentID(ctx, s.db, paymentID)
	if err != nil {
		return nil, err
	}

	return &SettlementReceiptResult{
		Payment:     *payment,
		Loan:        *loan,
		Member:      *member,
		Settlement:  *settlement,
		Allocations: allocations,
	}, nil
}
