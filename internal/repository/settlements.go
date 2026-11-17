package repository

import (
	"context"
	"database/sql"
	"errors"

	"koperasi/internal/domain"
)

type SettlementRepository struct{}

func NewSettlementRepository() *SettlementRepository {
	return &SettlementRepository{}
}

// Create menyimpan snapshot rincian pelunasan dipercepat ke tabel loan_settlements.
func (r *SettlementRepository) Create(ctx context.Context, db DBTX, s *domain.LoanSettlement) error {
	const query = `
		INSERT INTO loan_settlements (
			loan_id, payment_id, current_installment_id,
			arrears_amount, remaining_principal, current_interest,
			administration_fee, waived_interest
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := db.ExecContext(ctx, query,
		s.LoanID,
		s.PaymentID,
		s.CurrentInstallmentID,
		s.ArrearsAmount,
		s.RemainingPrincipal,
		s.CurrentInterest,
		s.AdministrationFee,
		s.WaivedInterest,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	return nil
}

// GetByLoanID mengambil record settlement berdasarkan ID pinjaman.
func (r *SettlementRepository) GetByLoanID(ctx context.Context, db DBTX, loanID int64) (*domain.LoanSettlement, error) {
	const query = `
		SELECT id, loan_id, payment_id, current_installment_id,
		       arrears_amount, remaining_principal, current_interest,
		       administration_fee, waived_interest
		FROM loan_settlements
		WHERE loan_id = ?
	`
	row := db.QueryRowContext(ctx, query, loanID)
	return scanSettlement(row)
}

// GetByPaymentID mengambil record settlement berdasarkan ID payment.
func (r *SettlementRepository) GetByPaymentID(ctx context.Context, db DBTX, paymentID int64) (*domain.LoanSettlement, error) {
	const query = `
		SELECT id, loan_id, payment_id, current_installment_id,
		       arrears_amount, remaining_principal, current_interest,
		       administration_fee, waived_interest
		FROM loan_settlements
		WHERE payment_id = ?
	`
	row := db.QueryRowContext(ctx, query, paymentID)
	return scanSettlement(row)
}

func scanSettlement(scanner interface{ Scan(dest ...any) error }) (*domain.LoanSettlement, error) {
	var s domain.LoanSettlement
	err := scanner.Scan(
		&s.ID,
		&s.LoanID,
		&s.PaymentID,
		&s.CurrentInstallmentID,
		&s.ArrearsAmount,
		&s.RemainingPrincipal,
		&s.CurrentInterest,
		&s.AdministrationFee,
		&s.WaivedInterest,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}
