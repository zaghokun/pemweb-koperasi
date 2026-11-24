package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"koperasi/internal/domain"
)

type InstallmentRepository struct{}

func NewInstallmentRepository() *InstallmentRepository {
	return &InstallmentRepository{}
}

// CreateBatch membuat baris-baris jadwal angsuran sekaligus saat pencairan pinjaman.
func (r *InstallmentRepository) CreateBatch(ctx context.Context, db DBTX, loanID int64, items []domain.Installment) error {
	const query = `
		INSERT INTO installments (
			loan_id, installment_number, due_date,
			principal_due, interest_due, interest_waived,
			waived_on, closed_on
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range items {
		item := &items[i]
		item.LoanID = loanID

		res, err := stmt.ExecContext(ctx,
			loanID,
			item.InstallmentNumber,
			item.DueDate.Format("2006-01-02"),
			item.PrincipalDue,
			item.InterestDue,
			item.InterestWaived,
			formatNullableDate(item.WaivedOn),
			formatNullableDate(item.ClosedOn),
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		item.ID = id
	}
	return nil
}

// GetByLoanID mengambil jadwal seluruh angsuran pinjaman terurut nomor angsuran.
func (r *InstallmentRepository) GetByLoanID(ctx context.Context, db DBTX, loanID int64) ([]domain.Installment, error) {
	const query = `
		SELECT id, loan_id, installment_number, due_date,
		       principal_due, interest_due, interest_waived,
		       waived_on, closed_on
		FROM installments
		WHERE loan_id = ?
		ORDER BY installment_number ASC
	`
	rows, err := db.QueryContext(ctx, query, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Installment
	for rows.Next() {
		inst, err := scanInstallment(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *inst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *InstallmentRepository) LockByLoanID(ctx context.Context, db DBTX, loanID int64) error {
	const query = `
		SELECT id FROM installments FORCE INDEX (uq_loan_installment_number)
		WHERE loan_id = ? ORDER BY installment_number FOR UPDATE
	`
	rows, err := db.QueryContext(ctx, query, loanID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}

// CloseInstallment menandai angsuran telah lunas dan membekukan pertumbuhan denda pada tanggal closedOn.
func (r *InstallmentRepository) CloseInstallment(ctx context.Context, db DBTX, id int64, closedOn time.Time) error {
	const query = `
		UPDATE installments
		SET closed_on = ?
		WHERE id = ? AND closed_on IS NULL
	`
	_, err := db.ExecContext(ctx, query, closedOn.Format("2006-01-02"), id)
	return err
}

// WaiveInterest mencatat pembebasan jasa mendatang pada pelunasan dipercepat (KD-07/KD-14).
func (r *InstallmentRepository) WaiveInterest(ctx context.Context, db DBTX, id int64, waivedAmount int64, waivedOn time.Time) error {
	const query = `
		UPDATE installments
		SET interest_waived = ?, waived_on = ?, closed_on = ?
		WHERE id = ?
	`
	_, err := db.ExecContext(ctx, query, waivedAmount, waivedOn.Format("2006-01-02"), waivedOn.Format("2006-01-02"), id)
	return err
}

// GetInstallmentBalances mengambil snapshot angsuran beserta alokasi pembayaran sampai tanggal asOf.
func (r *InstallmentRepository) GetInstallmentBalances(ctx context.Context, db DBTX, loanID int64, asOf time.Time) ([]domain.InstallmentBalance, error) {
	const query = `
		SELECT
			i.id, i.loan_id, i.installment_number, i.due_date,
			i.principal_due, i.interest_due, i.interest_waived,
			i.waived_on, i.closed_on,
			COALESCE(SUM(CASE WHEN pa.component_type = 'POKOK' AND (p.payment_date <= ? OR p.payment_date IS NULL) THEN pa.amount ELSE 0 END), 0) AS principal_paid,
			COALESCE(SUM(CASE WHEN pa.component_type = 'JASA' AND (p.payment_date <= ? OR p.payment_date IS NULL) THEN pa.amount ELSE 0 END), 0) AS interest_paid,
			COALESCE(SUM(CASE WHEN pa.component_type = 'DENDA' AND (p.payment_date <= ? OR p.payment_date IS NULL) THEN pa.amount ELSE 0 END), 0) AS penalty_paid
		FROM installments i
		LEFT JOIN payment_allocations pa ON pa.installment_id = i.id
		LEFT JOIN payments p ON p.id = pa.payment_id
		WHERE i.loan_id = ?
		GROUP BY i.id, i.loan_id, i.installment_number, i.due_date,
		         i.principal_due, i.interest_due, i.interest_waived,
		         i.waived_on, i.closed_on
		ORDER BY i.installment_number ASC
	`
	asOfStr := asOf.Format("2006-01-02")
	rows, err := db.QueryContext(ctx, query, asOfStr, asOfStr, asOfStr, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []domain.InstallmentBalance
	for rows.Next() {
		var ib domain.InstallmentBalance

		err := rows.Scan(
			&ib.Installment.ID,
			&ib.Installment.LoanID,
			&ib.Installment.InstallmentNumber,
			&ib.Installment.DueDate,
			&ib.Installment.PrincipalDue,
			&ib.Installment.InterestDue,
			&ib.Installment.InterestWaived,
			&ib.Installment.WaivedOn,
			&ib.Installment.ClosedOn,
			&ib.PrincipalPaid,
			&ib.InterestPaid,
			&ib.PenaltyPaid,
		)
		if err != nil {
			return nil, err
		}

		if ib.Installment.WaivedOn != nil && ib.Installment.WaivedOn.After(asOf) {
			ib.Installment.WaivedOn = nil
			ib.Installment.InterestWaived = 0
		}
		if ib.Installment.ClosedOn != nil && ib.Installment.ClosedOn.After(asOf) {
			ib.Installment.ClosedOn = nil
		}

		balances = append(balances, ib)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return balances, nil
}

func scanInstallment(scanner interface{ Scan(dest ...any) error }) (*domain.Installment, error) {
	var inst domain.Installment

	err := scanner.Scan(
		&inst.ID,
		&inst.LoanID,
		&inst.InstallmentNumber,
		&inst.DueDate,
		&inst.PrincipalDue,
		&inst.InterestDue,
		&inst.InterestWaived,
		&inst.WaivedOn,
		&inst.ClosedOn,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &inst, nil
}
