package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"koperasi/internal/domain"
)

type LoanRepository struct{}

func NewLoanRepository() *LoanRepository {
	return &LoanRepository{}
}

// Create menyimpan pengajuan pinjaman baru.
func (r *LoanRepository) Create(ctx context.Context, db DBTX, l *domain.Loan) error {
	const query = `
		INSERT INTO loans (
			member_id, principal, tenor_months, application_date,
			status, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	res, err := db.ExecContext(ctx, query,
		l.MemberID,
		l.Principal,
		l.TenorMonths,
		l.ApplicationDate.Format("2006-01-02"),
		string(l.Status),
		l.CreatedBy,
		l.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	l.ID = id
	return nil
}

// GetByID mengambil pinjaman berdasarkan PK id.
func (r *LoanRepository) GetByID(ctx context.Context, db DBTX, id int64) (*domain.Loan, error) {
	const query = `
		SELECT id, member_id, principal, tenor_months, application_date,
		       status, created_by, approved_by, disbursed_by, rejected_by,
		       disbursement_date, settled_on, rejection_reason, created_at
		FROM loans
		WHERE id = ?
	`
	row := db.QueryRowContext(ctx, query, id)
	return scanLoan(row)
}

// GetByIDForUpdate mengambil data pinjaman dengan row lock FOR UPDATE.
func (r *LoanRepository) GetByIDForUpdate(ctx context.Context, db DBTX, id int64) (*domain.Loan, error) {
	const query = `
		SELECT id, member_id, principal, tenor_months, application_date,
		       status, created_by, approved_by, disbursed_by, rejected_by,
		       disbursement_date, settled_on, rejection_reason, created_at
		FROM loans
		WHERE id = ?
		FOR UPDATE
	`
	row := db.QueryRowContext(ctx, query, id)
	return scanLoan(row)
}

// HasOpenLoan memeriksa apakah anggota masih memiliki pinjaman terbuka (PENDING, APPROVED, atau ACTIVE).
func (r *LoanRepository) HasOpenLoan(ctx context.Context, db DBTX, memberID int64) (bool, error) {
	const query = `
		SELECT EXISTS(
			SELECT 1 FROM loans
			WHERE member_id = ? AND status IN ('PENDING', 'APPROVED', 'ACTIVE')
		)
	`
	var exists bool
	if err := db.QueryRowContext(ctx, query, memberID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// UpdateStatus mengubah status pinjaman beserta metadata aksi (approval, disbursement, rejection, settlement).
func (r *LoanRepository) UpdateStatus(
	ctx context.Context,
	db DBTX,
	id int64,
	status domain.LoanStatus,
	approvedBy *int64,
	disbursedBy *int64,
	rejectedBy *int64,
	disbursementDate *time.Time,
	settledOn *time.Time,
	rejectionReason *string,
) error {
	const query = `
		UPDATE loans
		SET status = ?, approved_by = ?, disbursed_by = ?, rejected_by = ?,
		    disbursement_date = ?, settled_on = ?, rejection_reason = ?
		WHERE id = ?
	`
	res, err := db.ExecContext(ctx, query,
		string(status),
		approvedBy,
		disbursedBy,
		rejectedBy,
		formatNullableDate(disbursementDate),
		formatNullableDate(settledOn),
		rejectionReason,
		id,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// List mengambil daftar pinjaman dengan pagination dan filter opsional.
func (r *LoanRepository) List(ctx context.Context, db DBTX, memberID *int64, status *domain.LoanStatus, limit, offset int) ([]domain.Loan, int, error) {
	var conditions []string
	var args []any

	if memberID != nil {
		conditions = append(conditions, "member_id = ?")
		args = append(args, *memberID)
	}
	if status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, string(*status))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM loans " + whereClause
	var total int
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `
		SELECT id, member_id, principal, tenor_months, application_date,
		       status, created_by, approved_by, disbursed_by, rejected_by,
		       disbursement_date, settled_on, rejection_reason, created_at
		FROM loans ` + whereClause + `
		ORDER BY id DESC
		LIMIT ? OFFSET ?
	`
	queryArgs := append(args, limit, offset)
	rows, err := db.QueryContext(ctx, listQuery, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []domain.Loan
	for rows.Next() {
		l, err := scanLoan(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

func scanLoan(scanner interface{ Scan(dest ...any) error }) (*domain.Loan, error) {
	var l domain.Loan
	var statusStr string
	var approvedBy sql.NullInt64
	var disbursedBy sql.NullInt64
	var rejectedBy sql.NullInt64
	var rejectReason sql.NullString

	err := scanner.Scan(
		&l.ID,
		&l.MemberID,
		&l.Principal,
		&l.TenorMonths,
		&l.ApplicationDate,
		&statusStr,
		&l.CreatedBy,
		&approvedBy,
		&disbursedBy,
		&rejectedBy,
		&l.DisbursementDate,
		&l.SettledOn,
		&rejectReason,
		&l.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	l.Status = domain.LoanStatus(statusStr)

	if approvedBy.Valid {
		uid := approvedBy.Int64
		l.ApprovedBy = &uid
	}
	if disbursedBy.Valid {
		uid := disbursedBy.Int64
		l.DisbursedBy = &uid
	}
	if rejectedBy.Valid {
		uid := rejectedBy.Int64
		l.RejectedBy = &uid
	}

	if rejectReason.Valid {
		l.RejectionReason = &rejectReason.String
	}

	return &l, nil
}
