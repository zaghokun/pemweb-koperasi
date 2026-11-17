package repository

import (
	"context"
	"time"
)

type ReportRepository struct{}

func NewReportRepository() *ReportRepository {
	return &ReportRepository{}
}

type DashboardSummary struct {
	ActiveMemberCount         int64
	TotalPrincipalSavings     int64
	TotalMandatorySavings     int64
	TotalVoluntarySavings     int64
	TotalSavings              int64
	ActiveLoanCount           int64
	ActiveLoanPrincipal       int64
	ArrearsLoanCount          int64
	CurrentMonthInterestIncome int64
	CurrentMonthPenaltyIncome int64
}

type MonthlyIncomeSummary struct {
	Year                    int
	Month                   int
	PrincipalReceived       int64
	InterestIncome          int64
	PenaltyIncome           int64
	AdminFeeIncome          int64
	PrincipalSavingsReceived int64
	MandatorySavingsReceived int64
	VoluntarySavingsReceived int64
	TotalCashIn             int64
}

type ArrearsLoanSummary struct {
	LoanID                  int64
	MemberID                int64
	MemberNumber            string
	MemberName              string
	Phone                   string
	Principal               int64
	DisbursementDate        *time.Time
	OverdueInstallmentCount int
	MaxDaysOverdue          int
	PrincipalOverdue        int64
	InterestOverdue         int64
	PenaltyDue              int64
	TotalArrears            int64
}

// GetDashboardSummary mengambil indikator utama operasional koperasi untuk dashboard pengurus.
func (r *ReportRepository) GetDashboardSummary(ctx context.Context, db DBTX, asOfDate time.Time) (*DashboardSummary, error) {
	var s DashboardSummary
	asOfStr := asOfDate.Format("2006-01-02")

	// 1. Anggota aktif
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM members WHERE status = 'ACTIVE'").Scan(&s.ActiveMemberCount)
	if err != nil {
		return nil, err
	}

	// 2. Total simpanan bersih
	const querySavings = `
		SELECT
			COALESCE(SUM(CASE WHEN saving_type = 'POKOK' AND transaction_type = 'SETOR' THEN amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN saving_type = 'POKOK' AND transaction_type = 'PENGEMBALIAN_KELUAR' THEN amount ELSE 0 END), 0) AS pokok,
			COALESCE(SUM(CASE WHEN saving_type = 'WAJIB' AND transaction_type = 'SETOR' THEN amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN saving_type = 'WAJIB' AND transaction_type = 'PENGEMBALIAN_KELUAR' THEN amount ELSE 0 END), 0) AS wajib,
			COALESCE(SUM(CASE WHEN saving_type = 'SUKARELA' AND transaction_type = 'SETOR' THEN amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN saving_type = 'SUKARELA' AND transaction_type IN ('TARIK', 'PENGEMBALIAN_KELUAR') THEN amount ELSE 0 END), 0) AS sukarela
		FROM savings
	`
	err = db.QueryRowContext(ctx, querySavings).Scan(&s.TotalPrincipalSavings, &s.TotalMandatorySavings, &s.TotalVoluntarySavings)
	if err != nil {
		return nil, err
	}
	s.TotalSavings = s.TotalPrincipalSavings + s.TotalMandatorySavings + s.TotalVoluntarySavings

	// 3. Pinjaman aktif
	err = db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(principal), 0) FROM loans WHERE status = 'ACTIVE'").Scan(
		&s.ActiveLoanCount, &s.ActiveLoanPrincipal,
	)
	if err != nil {
		return nil, err
	}

	// 4. Pinjaman menunggak per asOfDate
	const queryArrearsCount = `
		SELECT COUNT(DISTINCT l.id)
		FROM loans l
		JOIN installments i ON i.loan_id = l.id
		WHERE l.status = 'ACTIVE'
		  AND (i.closed_on IS NULL OR i.closed_on > ?)
		  AND i.due_date < ?
	`
	err = db.QueryRowContext(ctx, queryArrearsCount, asOfStr, asOfStr).Scan(&s.ArrearsLoanCount)
	if err != nil {
		return nil, err
	}

	// 5. Pendapatan jasa & denda bulan berjalan
	startOfMonth := time.Date(asOfDate.Year(), asOfDate.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	const queryMonthIncome = `
		SELECT
			COALESCE(SUM(CASE WHEN pa.component_type = 'JASA' THEN pa.amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN pa.component_type = 'DENDA' THEN pa.amount ELSE 0 END), 0)
		FROM payment_allocations pa
		JOIN payments p ON p.id = pa.payment_id
		WHERE p.payment_date >= ? AND p.payment_date <= ?
	`
	err = db.QueryRowContext(ctx, queryMonthIncome, startOfMonth, asOfStr).Scan(
		&s.CurrentMonthInterestIncome, &s.CurrentMonthPenaltyIncome,
	)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

// GetMonthlyIncome merekapitulasi seluruh penerimaan kas masuk pada bulan tertentu (KD-11).
func (r *ReportRepository) GetMonthlyIncome(ctx context.Context, db DBTX, year int, month int) (*MonthlyIncomeSummary, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, -1)
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")

	summary := &MonthlyIncomeSummary{
		Year:  year,
		Month: month,
	}

	// 1. Alokasi pembayaran pinjaman
	const queryPay = `
		SELECT
			COALESCE(SUM(CASE WHEN pa.component_type = 'POKOK' THEN pa.amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN pa.component_type = 'JASA' THEN pa.amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN pa.component_type = 'DENDA' THEN pa.amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN pa.component_type = 'ADMIN' THEN pa.amount ELSE 0 END), 0)
		FROM payment_allocations pa
		JOIN payments p ON p.id = pa.payment_id
		WHERE p.payment_date >= ? AND p.payment_date <= ?
	`
	err := db.QueryRowContext(ctx, queryPay, startStr, endStr).Scan(
		&summary.PrincipalReceived,
		&summary.InterestIncome,
		&summary.PenaltyIncome,
		&summary.AdminFeeIncome,
	)
	if err != nil {
		return nil, err
	}

	// 2. Setoran simpanan masuk
	const querySavings = `
		SELECT
			COALESCE(SUM(CASE WHEN saving_type = 'POKOK' AND transaction_type = 'SETOR' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN saving_type = 'WAJIB' AND transaction_type = 'SETOR' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN saving_type = 'SUKARELA' AND transaction_type = 'SETOR' THEN amount ELSE 0 END), 0)
		FROM savings
		WHERE transaction_date >= ? AND transaction_date <= ?
	`
	err = db.QueryRowContext(ctx, querySavings, startStr, endStr).Scan(
		&summary.PrincipalSavingsReceived,
		&summary.MandatorySavingsReceived,
		&summary.VoluntarySavingsReceived,
	)
	if err != nil {
		return nil, err
	}

	summary.TotalCashIn = summary.PrincipalReceived +
		summary.InterestIncome +
		summary.PenaltyIncome +
		summary.AdminFeeIncome +
		summary.PrincipalSavingsReceived +
		summary.MandatorySavingsReceived +
		summary.VoluntarySavingsReceived

	return summary, nil
}

// GetActiveLoanIDsWithOverdue mengambil daftar pinjaman aktif yang berpotensi menunggak pada tanggal asOf.
func (r *ReportRepository) GetActiveLoanIDsWithOverdue(ctx context.Context, db DBTX, asOf time.Time) ([]int64, error) {
	asOfStr := asOf.Format("2006-01-02")
	const query = `
		SELECT DISTINCT l.id
		FROM loans l
		JOIN installments i ON i.loan_id = l.id
		WHERE l.status = 'ACTIVE'
		  AND (i.closed_on IS NULL OR i.closed_on > ?)
		  AND i.due_date < ?
		ORDER BY l.id ASC
	`
	rows, err := db.QueryContext(ctx, query, asOfStr, asOfStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
