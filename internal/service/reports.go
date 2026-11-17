package service

import (
	"context"
	"database/sql"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
)

type LoanCardResult struct {
	Loan             domain.Loan
	Member           domain.Member
	Installments     []domain.Installment
	Balances         []domain.InstallmentBalance
	Payments         []domain.Payment
	Allocations      []domain.PaymentAllocation
	Settlement       *domain.LoanSettlement
	TotalPaid        int64
	RemainingBalance int64
}

type ReportService struct {
	db              *sql.DB
	reportRepo      *repository.ReportRepository
	loanRepo        *repository.LoanRepository
	installmentRepo *repository.InstallmentRepository
	paymentRepo     *repository.PaymentRepository
	memberRepo      *repository.MemberRepository
	settlementRepo  *repository.SettlementRepository
}

func NewReportService(
	db *sql.DB,
	reportRepo *repository.ReportRepository,
	loanRepo *repository.LoanRepository,
	installmentRepo *repository.InstallmentRepository,
	paymentRepo *repository.PaymentRepository,
	memberRepo *repository.MemberRepository,
	settlementRepo *repository.SettlementRepository,
) *ReportService {
	return &ReportService{
		db:              db,
		reportRepo:      reportRepo,
		loanRepo:        loanRepo,
		installmentRepo: installmentRepo,
		paymentRepo:     paymentRepo,
		memberRepo:      memberRepo,
		settlementRepo:  settlementRepo,
	}
}

// DashboardStats mengembalikan agregat statistik operasional utama koperasi.
func (s *ReportService) DashboardStats(ctx context.Context, asOfDate time.Time) (*repository.DashboardSummary, error) {
	if s == nil || s.db == nil || s.reportRepo == nil {
		return &repository.DashboardSummary{}, nil
	}
	normDate, err := domain.NormalizeBusinessDate(asOfDate)
	if err != nil {
		return nil, err
	}
	return s.reportRepo.GetDashboardSummary(ctx, s.db, normDate)
}

// MonthlyIncome mengembalikan rekapitulasi penerimaan kas bulanan.
func (s *ReportService) MonthlyIncome(ctx context.Context, year int, month int) (*repository.MonthlyIncomeSummary, error) {
	if s == nil || s.db == nil || s.reportRepo == nil {
		return &repository.MonthlyIncomeSummary{Year: year, Month: month}, nil
	}
	if year < 2000 || year > 2100 || month < 1 || month > 12 {
		now := time.Now().UTC()
		year = now.Year()
		month = int(now.Month())
	}
	return s.reportRepo.GetMonthlyIncome(ctx, s.db, year, month)
}

// ArrearsReport menyusun daftar seluruh pinjaman aktif yang menunggak per tanggal asOf (KD-02, KD-03, KD-11).
func (s *ReportService) ArrearsReport(ctx context.Context, asOfDate time.Time) ([]repository.ArrearsLoanSummary, error) {
	if s == nil || s.db == nil || s.reportRepo == nil {
		return nil, nil
	}
	normDate, err := domain.NormalizeBusinessDate(asOfDate)
	if err != nil {
		return nil, err
	}

	loanIDs, err := s.reportRepo.GetActiveLoanIDsWithOverdue(ctx, s.db, normDate)
	if err != nil {
		return nil, err
	}

	var results []repository.ArrearsLoanSummary

	for _, loanID := range loanIDs {
		loan, err := s.loanRepo.GetByID(ctx, s.db, loanID)
		if err != nil {
			return nil, err
		}
		member, err := s.memberRepo.GetByID(ctx, s.db, loan.MemberID)
		if err != nil {
			return nil, err
		}
		balances, err := s.installmentRepo.GetInstallmentBalances(ctx, s.db, loanID, normDate)
		if err != nil {
			return nil, err
		}

		var overdueCount int
		var maxDays int
		var sumPrincipal, sumInterest, sumPenalty int64

		for _, b := range balances {
			due, err := domain.NormalizeBusinessDate(b.Installment.DueDate)
			if err != nil {
				continue
			}

			if due.Before(normDate) {
				days, penaltyRem, err := domain.CalculatePenalty(b, normDate)
				if err != nil {
					return nil, err
				}

				remPrincipal := b.Installment.PrincipalDue - b.PrincipalPaid
				if remPrincipal < 0 {
					remPrincipal = 0
				}
				remInterest := b.Installment.InterestDue - b.Installment.InterestWaived - b.InterestPaid
				if remInterest < 0 {
					remInterest = 0
				}

				if remPrincipal > 0 || remInterest > 0 || penaltyRem > 0 {
					overdueCount++
					if int(days) > maxDays {
						maxDays = int(days)
					}
					sumPrincipal += remPrincipal
					sumInterest += remInterest
					sumPenalty += penaltyRem
				}
			}
		}

		totalArrears := sumPrincipal + sumInterest + sumPenalty
		if totalArrears > 0 {
			results = append(results, repository.ArrearsLoanSummary{
				LoanID:                  loan.ID,
				MemberID:                member.ID,
				MemberNumber:            member.MemberNumber,
				MemberName:              member.Name,
				Phone:                   member.Phone,
				Principal:               loan.Principal,
				DisbursementDate:        loan.DisbursementDate,
				OverdueInstallmentCount: overdueCount,
				MaxDaysOverdue:          maxDays,
				PrincipalOverdue:        sumPrincipal,
				InterestOverdue:         sumInterest,
				PenaltyDue:              sumPenalty,
				TotalArrears:            totalArrears,
			})
		}
	}

	return results, nil
}

// LoanCard mengambil kartu pinjaman lengkap beserta jadwal dan histori pembayarannya.
func (s *ReportService) LoanCard(ctx context.Context, loanID int64) (*LoanCardResult, error) {
	loan, err := s.loanRepo.GetByID(ctx, s.db, loanID)
	if err != nil {
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
	balances, err := s.installmentRepo.GetInstallmentBalances(ctx, s.db, loanID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	payments, err := s.paymentRepo.ListByLoanID(ctx, s.db, loanID)
	if err != nil {
		return nil, err
	}

	var allAllocations []domain.PaymentAllocation
	for _, p := range payments {
		allocs, err := s.paymentRepo.GetAllocationsByPaymentID(ctx, s.db, p.ID)
		if err == nil {
			allAllocations = append(allAllocations, allocs...)
		}
	}

	settlement, _ := s.settlementRepo.GetByLoanID(ctx, s.db, loanID)

	var totalPaid int64
	for _, p := range payments {
		totalPaid += p.Amount
	}

	var principalPaidTotal int64
	for _, b := range balances {
		principalPaidTotal += b.PrincipalPaid
	}
	remPrincipal := loan.Principal - principalPaidTotal
	if remPrincipal < 0 {
		remPrincipal = 0
	}

	return &LoanCardResult{
		Loan:             *loan,
		Member:           *member,
		Installments:     installments,
		Balances:         balances,
		Payments:         payments,
		Allocations:      allAllocations,
		Settlement:       settlement,
		TotalPaid:        totalPaid,
		RemainingBalance: remPrincipal,
	}, nil
}
