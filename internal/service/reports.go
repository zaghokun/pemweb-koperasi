package service

import (
	"context"
	"database/sql"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
)

type LoanCardResult struct {
	Loan                domain.Loan
	Member              domain.Member
	Balances            []domain.InstallmentBalance
	Payments            []domain.Payment
	Status              string
	MonthlyInterest     int64
	RemainingPrincipal  int64
	RemainingObligation int64
}

type ReportService struct {
	db              *sql.DB
	reportRepo      *repository.ReportRepository
	loanRepo        *repository.LoanRepository
	installmentRepo *repository.InstallmentRepository
	paymentRepo     *repository.PaymentRepository
	memberRepo      *repository.MemberRepository
}

func NewReportService(
	db *sql.DB,
	reportRepo *repository.ReportRepository,
	loanRepo *repository.LoanRepository,
	installmentRepo *repository.InstallmentRepository,
	paymentRepo *repository.PaymentRepository,
	memberRepo *repository.MemberRepository,
) *ReportService {
	return &ReportService{
		db:              db,
		reportRepo:      reportRepo,
		loanRepo:        loanRepo,
		installmentRepo: installmentRepo,
		paymentRepo:     paymentRepo,
		memberRepo:      memberRepo,
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
		now := BusinessToday()
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

	loanIDs, err := s.reportRepo.GetLoanIDsWithOverdue(ctx, s.db, normDate)
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
				penalty, penaltyRem, err := domain.CalculatePenalty(b, normDate)
				days := int(penalty / domain.DailyPenaltyAmount)
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
					if days > maxDays {
						maxDays = days
					}
					sumPrincipal, err = domain.AddMoney(sumPrincipal, remPrincipal)
					if err != nil {
						return nil, err
					}
					sumInterest, err = domain.AddMoney(sumInterest, remInterest)
					if err != nil {
						return nil, err
					}
					sumPenalty, err = domain.AddMoney(sumPenalty, penaltyRem)
					if err != nil {
						return nil, err
					}
				}
			}
		}

		totalArrears, err := domain.TotalObligation([]domain.Obligation{{Principal: sumPrincipal, Interest: sumInterest, Penalty: sumPenalty}})
		if err != nil {
			return nil, err
		}
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
func (s *ReportService) LoanCard(ctx context.Context, loanID int64, asOf time.Time) (*LoanCardResult, error) {
	loan, err := s.loanRepo.GetByID(ctx, s.db, loanID)
	if err != nil {
		return nil, err
	}
	member, err := s.memberRepo.GetByID(ctx, s.db, loan.MemberID)
	if err != nil {
		return nil, err
	}
	card := &LoanCardResult{Loan: *loan, Member: *member}
	switch {
	case asOf.Before(loan.ApplicationDate):
		card.Status = "Belum diajukan"
	case loan.DisbursementDate != nil && asOf.Before(*loan.DisbursementDate):
		card.Status = "Belum cair"
	case loan.DisbursementDate == nil && loan.Status == domain.LoanRejected:
		card.Status = "Ditolak"
	case loan.DisbursementDate == nil:
		card.Status = "Belum cair"
	case loan.SettledOn != nil && !loan.SettledOn.After(asOf):
		card.Status = "Lunas"
	default:
		card.Status = "Aktif"
	}
	if card.Loan.DisbursementDate == nil || card.Loan.DisbursementDate.After(asOf) {
		card.Loan.DisbursementDate = nil
		return card, nil
	}

	card.Balances, err = s.installmentRepo.GetInstallmentBalances(ctx, s.db, loanID, asOf)
	if err != nil {
		return nil, err
	}
	if len(card.Balances) != 0 {
		card.MonthlyInterest = card.Balances[0].Installment.InterestDue
	}

	obligations := make([]domain.Obligation, 0, len(card.Balances))
	for _, balance := range card.Balances {
		_, penalty, err := domain.CalculatePenalty(balance, asOf)
		if err != nil {
			return nil, err
		}
		principal := balance.Installment.PrincipalDue - balance.PrincipalPaid
		interest := balance.Installment.InterestDue - balance.Installment.InterestWaived - balance.InterestPaid
		obligations = append(obligations, domain.Obligation{Principal: principal, Interest: interest, Penalty: penalty})
		card.RemainingPrincipal, err = domain.AddMoney(card.RemainingPrincipal, principal)
		if err != nil {
			return nil, err
		}
	}
	card.RemainingObligation, err = domain.TotalObligation(obligations)
	if err != nil {
		return nil, err
	}

	payments, err := s.paymentRepo.ListByLoanID(ctx, s.db, loanID)
	if err != nil {
		return nil, err
	}
	for _, payment := range payments {
		if !payment.PaymentDate.After(asOf) {
			card.Payments = append(card.Payments, payment)
		}
	}
	return card, nil
}
