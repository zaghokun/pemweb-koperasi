package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"koperasi/internal/domain"
)

type SavingRepository struct{}

func NewSavingRepository() *SavingRepository {
	return &SavingRepository{}
}

// Create mencatat transaksi simpanan baru.
func (r *SavingRepository) Create(ctx context.Context, db DBTX, s *domain.Saving) error {
	const query = `
		INSERT INTO savings (
			member_id, user_id, saving_type, transaction_type, amount,
			period_month, transaction_date, request_key, description, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := db.ExecContext(ctx, query,
		s.MemberID,
		s.UserID,
		string(s.SavingType),
		string(s.TransactionType),
		s.Amount,
		formatNullableDate(s.PeriodMonth),
		s.TransactionDate.Format("2006-01-02"),
		strings.TrimSpace(s.RequestKey),
		s.Description,
		s.CreatedAt.UTC(),
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

// GetByID mengambil record simpanan berdasarkan PK id.
func (r *SavingRepository) GetByID(ctx context.Context, db DBTX, id int64) (*domain.Saving, error) {
	const query = `
		SELECT id, member_id, user_id, saving_type, transaction_type, amount,
		       period_month, transaction_date, request_key, description, created_at
		FROM savings
		WHERE id = ?
	`
	row := db.QueryRowContext(ctx, query, id)
	return scanSaving(row)
}

// GetByRequestKey mencari simpanan berdasarkan request_key untuk idempotensi.
func (r *SavingRepository) GetByRequestKey(ctx context.Context, db DBTX, key string) (*domain.Saving, error) {
	const query = `
		SELECT id, member_id, user_id, saving_type, transaction_type, amount,
		       period_month, transaction_date, request_key, description, created_at
		FROM savings
		WHERE request_key = ?
	`
	row := db.QueryRowContext(ctx, query, strings.TrimSpace(key))
	return scanSaving(row)
}

// GetBalances menghitung saldo pokok, wajib, dan sukarela dari agregat transaksi tanpa kolom saldo terduplikasi.
// Rumus: saldo = total SETOR - total TARIK - total PENGEMBALIAN_KELUAR.
func (r *SavingRepository) GetBalances(ctx context.Context, db DBTX, memberID int64) (pokok int64, wajib int64, sukarela int64, err error) {
	const query = `
		SELECT
			saving_type,
			COALESCE(SUM(CASE WHEN transaction_type = 'SETOR' THEN amount ELSE -amount END), 0) AS balance
		FROM savings
		WHERE member_id = ?
		GROUP BY saving_type
	`
	rows, err := db.QueryContext(ctx, query, memberID)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var st string
		var bal int64
		if err := rows.Scan(&st, &bal); err != nil {
			return 0, 0, 0, err
		}
		switch domain.SavingType(st) {
		case domain.SavingPrincipal:
			pokok = bal
		case domain.SavingMandatory:
			wajib = bal
		case domain.SavingVoluntary:
			sukarela = bal
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}

	return pokok, wajib, sukarela, nil
}

// ListByMemberID mengambil riwayat transaksi simpanan anggota terurut terbaru.
func (r *SavingRepository) ListByMemberID(ctx context.Context, db DBTX, memberID int64, limit, offset int) ([]domain.Saving, error) {
	const query = `
		SELECT id, member_id, user_id, saving_type, transaction_type, amount,
		       period_month, transaction_date, request_key, description, created_at
		FROM savings
		WHERE member_id = ?
		ORDER BY transaction_date DESC, id DESC
		LIMIT ? OFFSET ?
	`
	rows, err := db.QueryContext(ctx, query, memberID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Saving
	for rows.Next() {
		s, err := scanSaving(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// List mengambil riwayat simpanan dengan filter opsional.
func (r *SavingRepository) List(ctx context.Context, db DBTX, memberID *int64, savingType *domain.SavingType, limit, offset int) ([]domain.Saving, int, error) {
	var conditions []string
	var args []any

	if memberID != nil {
		conditions = append(conditions, "member_id = ?")
		args = append(args, *memberID)
	}
	if savingType != nil {
		conditions = append(conditions, "saving_type = ?")
		args = append(args, string(*savingType))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM savings " + whereClause
	var total int
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `
		SELECT id, member_id, user_id, saving_type, transaction_type, amount,
		       period_month, transaction_date, request_key, description, created_at
		FROM savings ` + whereClause + `
		ORDER BY transaction_date DESC, id DESC
		LIMIT ? OFFSET ?
	`
	queryArgs := append(args, limit, offset)
	rows, err := db.QueryContext(ctx, listQuery, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []domain.Saving
	for rows.Next() {
		s, err := scanSaving(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

func scanSaving(scanner interface{ Scan(dest ...any) error }) (*domain.Saving, error) {
	var s domain.Saving
	var st string
	var tt string
	var desc sql.NullString

	err := scanner.Scan(
		&s.ID,
		&s.MemberID,
		&s.UserID,
		&st,
		&tt,
		&s.Amount,
		&s.PeriodMonth,
		&s.TransactionDate,
		&s.RequestKey,
		&desc,
		&s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if desc.Valid {
		s.Description = &desc.String
	}

	s.SavingType = domain.SavingType(st)
	s.TransactionType = domain.SavingTransactionType(tt)

	return &s, nil
}
