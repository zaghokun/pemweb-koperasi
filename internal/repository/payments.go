package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"koperasi/internal/domain"
)

type PaymentRepository struct{}

func NewPaymentRepository() *PaymentRepository {
	return &PaymentRepository{}
}

// Create menyimpan header pembayaran baru.
func (r *PaymentRepository) Create(ctx context.Context, db DBTX, p *domain.Payment) error {
	const query = `
		INSERT INTO payments (
			loan_id, user_id, payment_type, payment_date,
			amount, request_key, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	res, err := db.ExecContext(ctx, query,
		p.LoanID,
		p.UserID,
		string(p.PaymentType),
		p.PaymentDate.Format("2006-01-02"),
		p.Amount,
		strings.TrimSpace(p.RequestKey),
		p.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = id
	return nil
}

// GetByID mengambil record payment berdasarkan PK id.
func (r *PaymentRepository) GetByID(ctx context.Context, db DBTX, id int64) (*domain.Payment, error) {
	const query = `
		SELECT id, loan_id, user_id, payment_type, payment_date,
		       amount, request_key, created_at
		FROM payments
		WHERE id = ?
	`
	row := db.QueryRowContext(ctx, query, id)
	return scanPayment(row)
}

// GetByRequestKey mencari payment berdasarkan request_key untuk idempotensi.
func (r *PaymentRepository) GetByRequestKey(ctx context.Context, db DBTX, key string) (*domain.Payment, error) {
	const query = `
		SELECT id, loan_id, user_id, payment_type, payment_date,
		       amount, request_key, created_at
		FROM payments
		WHERE request_key = ?
	`
	row := db.QueryRowContext(ctx, query, strings.TrimSpace(key))
	return scanPayment(row)
}

// CreateAllocations menyimpan seluruh rincian alokasi pembayaran.
func (r *PaymentRepository) CreateAllocations(ctx context.Context, db DBTX, paymentID int64, items []domain.PaymentAllocation) error {
	const query = `
		INSERT INTO payment_allocations (payment_id, installment_id, component_type, amount)
		VALUES (?, ?, ?, ?)
	`
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range items {
		item := &items[i]
		item.PaymentID = paymentID

		res, err := stmt.ExecContext(ctx,
			paymentID,
			item.InstallmentID,
			string(item.ComponentType),
			item.Amount,
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

// GetAllocationsByPaymentID mengambil seluruh rincian alokasi komponen untuk satu payment.
func (r *PaymentRepository) GetAllocationsByPaymentID(ctx context.Context, db DBTX, paymentID int64) ([]domain.PaymentAllocation, error) {
	const query = `
		SELECT id, payment_id, installment_id, component_type, amount
		FROM payment_allocations
		WHERE payment_id = ?
		ORDER BY id ASC
	`
	rows, err := db.QueryContext(ctx, query, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.PaymentAllocation
	for rows.Next() {
		var pa domain.PaymentAllocation
		var ct string
		var instID sql.NullInt64

		if err := rows.Scan(&pa.ID, &pa.PaymentID, &instID, &ct, &pa.Amount); err != nil {
			return nil, err
		}
		pa.ComponentType = domain.ComponentType(ct)
		if instID.Valid {
			idVal := instID.Int64
			pa.InstallmentID = &idVal
		}
		list = append(list, pa)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// ListByLoanID mengambil daftar seluruh pembayaran pinjaman terurut tanggal pembayaran terbaru.
func (r *PaymentRepository) ListByLoanID(ctx context.Context, db DBTX, loanID int64) ([]domain.Payment, error) {
	const query = `
		SELECT id, loan_id, user_id, payment_type, payment_date,
		       amount, request_key, created_at
		FROM payments
		WHERE loan_id = ?
		ORDER BY payment_date DESC, id DESC
	`
	rows, err := db.QueryContext(ctx, query, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func scanPayment(scanner interface{ Scan(dest ...any) error }) (*domain.Payment, error) {
	var p domain.Payment
	var pt string

	err := scanner.Scan(
		&p.ID,
		&p.LoanID,
		&p.UserID,
		&pt,
		&p.PaymentDate,
		&p.Amount,
		&p.RequestKey,
		&p.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	p.PaymentType = domain.PaymentType(pt)

	return &p, nil
}
