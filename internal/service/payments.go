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
	ErrPaymentNotFound    = errors.New("data pembayaran tidak ditemukan")
	ErrPaymentBeforeDisb  = errors.New("tanggal pembayaran tidak boleh mendahului tanggal pencairan pinjaman")
	ErrNoPayableObligation = errors.New("tidak ada kewajiban angsuran yang dapat dibayar pada tanggal ini")
)

type PayInput struct {
	LoanID      int64
	UserID      int64
	Amount      int64
	PaymentDate time.Time
	RequestKey  string
}

type PayResult struct {
	Payment      domain.Payment
	Allocations  []domain.PaymentAllocation
	ClosedCount  int
	IsLoanSettled bool
}

type PaymentPreview struct {
	Loan            domain.Loan
	Member          domain.Member
	CurrentPeriod   int
	Obligations     []domain.Obligation
	TotalPenalty    int64
	TotalInterest   int64
	TotalPrincipal  int64
	TotalObligation int64
}

type PaymentReceipt struct {
	Payment     domain.Payment
	Loan        domain.Loan
	Member      domain.Member
	Allocations []domain.PaymentAllocation
}

type PaymentService struct {
	db              *sql.DB
	paymentRepo     *repository.PaymentRepository
	loanRepo        *repository.LoanRepository
	installmentRepo *repository.InstallmentRepository
	memberRepo      *repository.MemberRepository
}

func NewPaymentService(
	db *sql.DB,
	paymentRepo *repository.PaymentRepository,
	loanRepo *repository.LoanRepository,
	installmentRepo *repository.InstallmentRepository,
	memberRepo *repository.MemberRepository,
) *PaymentService {
	return &PaymentService{
		db:              db,
		paymentRepo:     paymentRepo,
		loanRepo:        loanRepo,
		installmentRepo: installmentRepo,
		memberRepo:      memberRepo,
	}
}

// Preview menghitung kewajiban angsuran yang boleh dibayar (jatuh tempo lewat + periode berjalan)
// beserta denda harian per tanggal perhitungan (KD-02, KD-05, KD-06).
func (s *PaymentService) Preview(ctx context.Context, loanID int64, paymentDate time.Time) (*PaymentPreview, error) {
	normPayDate, err := domain.NormalizeBusinessDate(paymentDate)
	if err != nil {
		return nil, err
	}

	loan, err := s.loanRepo.GetByID(ctx, s.db, loanID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrLoanNotFound
		}
		return nil, err
	}

	if loan.Status != domain.LoanActive {
		return nil, ErrInvalidLoanStatus
	}

	member, err := s.memberRepo.GetByID(ctx, s.db, loan.MemberID)
	if err != nil {
		return nil, err
	}

	if loan.DisbursementDate == nil || normPayDate.Before(*loan.DisbursementDate) {
		return nil, ErrPaymentBeforeDisb
	}

	// Tentukan periode berjalan
	currPeriod, err := domain.CurrentPeriod(*loan.DisbursementDate, loan.TenorMonths, normPayDate)
	if err != nil && !errors.Is(err, domain.ErrOutsideSchedule) {
		return nil, err
	}
	// Jika setelah jatuh tempo terakhir, seluruh angsuran boleh dibayar
	if errors.Is(err, domain.ErrOutsideSchedule) && normPayDate.After(loan.DisbursementDate.AddDate(0, loan.TenorMonths, 0)) {
		currPeriod = loan.TenorMonths
	}

	balances, err := s.installmentRepo.GetInstallmentBalances(ctx, s.db, loanID, normPayDate)
	if err != nil {
		return nil, err
	}

	var obligations []domain.Obligation
	var totPen, totInt, totPrinc int64

	for _, bal := range balances {
		// Hanya angsuran sampai periode berjalan yang boleh dibayar lewat pembayaran biasa (KD-05)
		if bal.Installment.InstallmentNumber > currPeriod && currPeriod > 0 {
			continue
		}
		if bal.Installment.ClosedOn != nil {
			continue
		}

		_, remPenalty, err := domain.CalculatePenalty(bal, normPayDate)
		if err != nil {
			return nil, err
		}

		remInterest, err := domain.SubtractMoney(bal.Installment.InterestDue, bal.InterestPaid)
		if err != nil {
			return nil, err
		}
		remInterest, err = domain.SubtractMoney(remInterest, bal.Installment.InterestWaived)
		if err != nil {
			return nil, err
		}

		remPrincipal, err := domain.SubtractMoney(bal.Installment.PrincipalDue, bal.PrincipalPaid)
		if err != nil {
			return nil, err
		}

		ob := domain.Obligation{
			InstallmentID:     bal.Installment.ID,
			InstallmentNumber: bal.Installment.InstallmentNumber,
			Penalty:           remPenalty,
			Interest:          remInterest,
			Principal:         remPrincipal,
		}
		obligations = append(obligations, ob)

		totPen, _ = domain.AddMoney(totPen, remPenalty)
		totInt, _ = domain.AddMoney(totInt, remInterest)
		totPrinc, _ = domain.AddMoney(totPrinc, remPrincipal)
	}

	totObligation, err := domain.TotalObligation(obligations)
	if err != nil {
		return nil, err
	}

	return &PaymentPreview{
		Loan:            *loan,
		Member:          *member,
		CurrentPeriod:   currPeriod,
		Obligations:     obligations,
		TotalPenalty:    totPen,
		TotalInterest:   totInt,
		TotalPrincipal:  totPrinc,
		TotalObligation: totObligation,
	}, nil
}

// Pay memproses pembayaran biasa (penuh atau sebagian) dengan prioritas global denda -> jasa -> pokok
// dan penutupan angsuran yang lunas secara atomik (KD-03, KD-04, KD-05).
func (s *PaymentService) Pay(ctx context.Context, input PayInput) (*PayResult, error) {
	if input.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	reqKey := strings.TrimSpace(input.RequestKey)
	if reqKey == "" {
		return nil, errors.New("request key diperlukan untuk pembayaran")
	}

	normPayDate, err := domain.NormalizeBusinessDate(input.PaymentDate)
	if err != nil {
		return nil, err
	}

	// Idempotency check
	existing, err := s.paymentRepo.GetByRequestKey(ctx, s.db, reqKey)
	if err == nil && existing != nil {
		if existing.LoanID == input.LoanID && existing.Amount == input.Amount {
			allocs, err := s.paymentRepo.GetAllocationsByPaymentID(ctx, s.db, existing.ID)
			if err != nil {
				return nil, err
			}
			return &PayResult{
				Payment:     *existing,
				Allocations: allocs,
			}, nil
		}
		return nil, ErrRequestKeyConflict
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Kunci anggota lalu pinjaman FOR UPDATE (urutan serialisasi aman)
	loan, err := s.loanRepo.GetByIDForUpdate(ctx, tx, input.LoanID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrLoanNotFound
		}
		return nil, err
	}

	if loan.Status != domain.LoanActive {
		return nil, ErrInvalidLoanStatus
	}

	member, err := s.memberRepo.GetByIDForUpdate(ctx, tx, loan.MemberID)
	if err != nil {
		return nil, err
	}
	_ = member

	if loan.DisbursementDate == nil || normPayDate.Before(*loan.DisbursementDate) {
		return nil, ErrPaymentBeforeDisb
	}

	// Tentukan periode berjalan
	currPeriod, err := domain.CurrentPeriod(*loan.DisbursementDate, loan.TenorMonths, normPayDate)
	if err != nil && !errors.Is(err, domain.ErrOutsideSchedule) {
		return nil, err
	}
	if errors.Is(err, domain.ErrOutsideSchedule) && normPayDate.After(loan.DisbursementDate.AddDate(0, loan.TenorMonths, 0)) {
		currPeriod = loan.TenorMonths
	}

	balances, err := s.installmentRepo.GetInstallmentBalances(ctx, tx, input.LoanID, normPayDate)
	if err != nil {
		return nil, err
	}

	var obligations []domain.Obligation
	for _, bal := range balances {
		if bal.Installment.InstallmentNumber > currPeriod && currPeriod > 0 {
			continue
		}
		if bal.Installment.ClosedOn != nil {
			continue
		}

		_, remPenalty, err := domain.CalculatePenalty(bal, normPayDate)
		if err != nil {
			return nil, err
		}

		remInterest, err := domain.SubtractMoney(bal.Installment.InterestDue, bal.InterestPaid)
		if err != nil {
			return nil, err
		}
		remInterest, err = domain.SubtractMoney(remInterest, bal.Installment.InterestWaived)
		if err != nil {
			return nil, err
		}

		remPrincipal, err := domain.SubtractMoney(bal.Installment.PrincipalDue, bal.PrincipalPaid)
		if err != nil {
			return nil, err
		}

		ob := domain.Obligation{
			InstallmentID:     bal.Installment.ID,
			InstallmentNumber: bal.Installment.InstallmentNumber,
			Penalty:           remPenalty,
			Interest:          remInterest,
			Principal:         remPrincipal,
		}
		obligations = append(obligations, ob)
	}

	if len(obligations) == 0 {
		return nil, ErrNoPayableObligation
	}

	// Alokasikan pembayaran dengan kalkulator domain murni (denda -> jasa -> pokok)
	allocations, err := domain.AllocatePayment(obligations, input.Amount)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	payment := &domain.Payment{
		LoanID:      input.LoanID,
		UserID:      input.UserID,
		PaymentType: domain.PaymentRegular,
		PaymentDate: normPayDate,
		Amount:      input.Amount,
		RequestKey:  reqKey,
		CreatedAt:   now,
	}

	if err := s.paymentRepo.Create(ctx, tx, payment); err != nil {
		return nil, err
	}

	if err := s.paymentRepo.CreateAllocations(ctx, tx, payment.ID, allocations); err != nil {
		return nil, err
	}

	// Evaluasi penutupan angsuran yang lunas penuh
	// Peta akumulasi alokasi per angsuran
	allocByInst := make(map[int64]map[domain.ComponentType]int64)
	for _, a := range allocations {
		if a.InstallmentID == nil {
			continue
		}
		instID := *a.InstallmentID
		if allocByInst[instID] == nil {
			allocByInst[instID] = make(map[domain.ComponentType]int64)
		}
		allocByInst[instID][a.ComponentType] += a.Amount
	}

	closedCount := 0
	for _, ob := range obligations {
		allocs := allocByInst[ob.InstallmentID]
		remPen := ob.Penalty - allocs[domain.ComponentPenalty]
		remInt := ob.Interest - allocs[domain.ComponentInterest]
		remPrinc := ob.Principal - allocs[domain.ComponentPrincipal]

		// Jika seluruh sisa denda, jasa, dan pokok lunas, tutup angsuran (KD-03)
		if remPen == 0 && remInt == 0 && remPrinc == 0 {
			if err := s.installmentRepo.CloseInstallment(ctx, tx, ob.InstallmentID, normPayDate); err != nil {
				return nil, err
			}
			closedCount++
		}
	}

	// Periksa apakah semua angsuran pinjaman telah ditutup
	allInstallments, err := s.installmentRepo.GetByLoanID(ctx, tx, input.LoanID)
	if err != nil {
		return nil, err
	}
	allClosed := true
	for _, inst := range allInstallments {
		if inst.ClosedOn == nil {
			// Periksa apakah baru saja ditutup di loop atas
			allocs := allocByInst[inst.ID]
			// Cari obligation
			found := false
			for _, ob := range obligations {
				if ob.InstallmentID == inst.ID {
					found = true
					if (ob.Penalty-allocs[domain.ComponentPenalty]) != 0 ||
						(ob.Interest-allocs[domain.ComponentInterest]) != 0 ||
						(ob.Principal-allocs[domain.ComponentPrincipal]) != 0 {
						allClosed = false
					}
					break
				}
			}
			if !found {
				allClosed = false
			}
		}
	}

	isSettled := false
	if allClosed {
		if err := s.loanRepo.UpdateStatus(ctx, tx, input.LoanID, domain.LoanSettled, nil, nil, nil, nil, &normPayDate, nil); err != nil {
			return nil, err
		}
		isSettled = true
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &PayResult{
		Payment:       *payment,
		Allocations:   allocations,
		ClosedCount:   closedCount,
		IsLoanSettled: isSettled,
	}, nil
}

// GetReceipt mengambil data lengkap pembayaran, pinjaman, dan alokasi untuk kuitansi resmi.
func (s *PaymentService) GetReceipt(ctx context.Context, paymentID int64) (*PaymentReceipt, error) {
	payment, err := s.paymentRepo.GetByID(ctx, s.db, paymentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	loan, err := s.loanRepo.GetByID(ctx, s.db, payment.LoanID)
	if err != nil {
		return nil, err
	}

	member, err := s.memberRepo.GetByID(ctx, s.db, loan.MemberID)
	if err != nil {
		return nil, err
	}

	allocations, err := s.paymentRepo.GetAllocationsByPaymentID(ctx, s.db, payment.ID)
	if err != nil {
		return nil, err
	}

	return &PaymentReceipt{
		Payment:     *payment,
		Loan:        *loan,
		Member:      *member,
		Allocations: allocations,
	}, nil
}
