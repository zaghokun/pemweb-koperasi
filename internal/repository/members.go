package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"koperasi/internal/domain"
)

type MemberRepository struct{}

func NewMemberRepository() *MemberRepository {
	return &MemberRepository{}
}

// Create menambahkan anggota baru.
func (r *MemberRepository) Create(ctx context.Context, db DBTX, m *domain.Member) error {
	const query = `
		INSERT INTO members (
			member_number, name, address, phone, join_date,
			status, exit_date, exit_user_id, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := db.ExecContext(ctx, query,
		strings.TrimSpace(m.MemberNumber),
		strings.TrimSpace(m.Name),
		strings.TrimSpace(m.Address),
		strings.TrimSpace(m.Phone),
		m.JoinDate.Format("2006-01-02"),
		string(m.Status),
		formatNullableDate(m.ExitDate),
		m.ExitUserID,
		m.CreatedBy,
		m.CreatedAt.UTC(),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	m.ID = id
	return nil
}

// GetByID mengambil anggota berdasarkan PK id.
func (r *MemberRepository) GetByID(ctx context.Context, db DBTX, id int64) (*domain.Member, error) {
	const query = `
		SELECT id, member_number, name, address, phone, join_date,
		       status, exit_date, exit_user_id, created_by, created_at
		FROM members
		WHERE id = ?
	`
	row := db.QueryRowContext(ctx, query, id)
	return scanMember(row)
}

// GetByIDForUpdate mengambil data anggota dengan row lock FOR UPDATE (serialisasi mutasi keuangan).
func (r *MemberRepository) GetByIDForUpdate(ctx context.Context, db DBTX, id int64) (*domain.Member, error) {
	const query = `
		SELECT id, member_number, name, address, phone, join_date,
		       status, exit_date, exit_user_id, created_by, created_at
		FROM members
		WHERE id = ?
		FOR UPDATE
	`
	row := db.QueryRowContext(ctx, query, id)
	return scanMember(row)
}

// GetByNumber mengambil anggota berdasarkan member_number.
func (r *MemberRepository) GetByNumber(ctx context.Context, db DBTX, number string) (*domain.Member, error) {
	const query = `
		SELECT id, member_number, name, address, phone, join_date,
		       status, exit_date, exit_user_id, created_by, created_at
		FROM members
		WHERE member_number = ?
	`
	row := db.QueryRowContext(ctx, query, strings.TrimSpace(number))
	return scanMember(row)
}

// UpdateIdentity memperbarui nama, alamat, dan telepon tanpa mengubah nomor anggota atau histori keuangan.
func (r *MemberRepository) UpdateIdentity(ctx context.Context, db DBTX, id int64, name, address, phone string) error {
	const query = `
		UPDATE members
		SET name = ?, address = ?, phone = ?
		WHERE id = ?
	`
	res, err := db.ExecContext(ctx, query, strings.TrimSpace(name), strings.TrimSpace(address), strings.TrimSpace(phone), id)
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

// UpdateExit menandai anggota keluar (status EXITED, exit_date, exit_user_id).
func (r *MemberRepository) UpdateExit(ctx context.Context, db DBTX, id int64, exitDate time.Time, exitUserID int64) error {
	const query = `
		UPDATE members
		SET status = 'EXITED', exit_date = ?, exit_user_id = ?
		WHERE id = ? AND status = 'ACTIVE'
	`
	res, err := db.ExecContext(ctx, query, exitDate.Format("2006-01-02"), exitUserID, id)
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

// List mengambil daftar anggota dengan pencarian (nama atau nomor anggota) dan paginasi.
func (r *MemberRepository) List(ctx context.Context, db DBTX, search string, limit, offset int) ([]domain.Member, int, error) {
	search = strings.TrimSpace(search)
	var countQuery string
	var listQuery string
	var args []any

	if search != "" {
		likePattern := "%" + search + "%"
		countQuery = `SELECT COUNT(*) FROM members WHERE member_number LIKE ? OR name LIKE ?`
		listQuery = `
			SELECT id, member_number, name, address, phone, join_date,
			       status, exit_date, exit_user_id, created_by, created_at
			FROM members
			WHERE member_number LIKE ? OR name LIKE ?
			ORDER BY id DESC
			LIMIT ? OFFSET ?
		`
		args = []any{likePattern, likePattern}
	} else {
		countQuery = `SELECT COUNT(*) FROM members`
		listQuery = `
			SELECT id, member_number, name, address, phone, join_date,
			       status, exit_date, exit_user_id, created_by, created_at
			FROM members
			ORDER BY id DESC
			LIMIT ? OFFSET ?
		`
	}

	var total int
	var countRow *sql.Row
	if len(args) > 0 {
		countRow = db.QueryRowContext(ctx, countQuery, args...)
	} else {
		countRow = db.QueryRowContext(ctx, countQuery)
	}
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append(args, limit, offset)
	rows, err := db.QueryContext(ctx, listQuery, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var members []domain.Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, 0, err
		}
		members = append(members, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return members, total, nil
}

func scanMember(scanner interface{ Scan(dest ...any) error }) (*domain.Member, error) {
	var m domain.Member
	var statusStr string
	var exitDateStr sql.NullString
	var exitUserID sql.NullInt64
	var joinDateStr string

	err := scanner.Scan(
		&m.ID,
		&m.MemberNumber,
		&m.Name,
		&m.Address,
		&m.Phone,
		&joinDateStr,
		&statusStr,
		&exitDateStr,
		&exitUserID,
		&m.CreatedBy,
		&m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	m.Status = domain.MemberStatus(statusStr)
	jt, err := domain.ParseBusinessDate(joinDateStr)
	if err != nil {
		return nil, err
	}
	m.JoinDate = jt

	if exitDateStr.Valid && exitDateStr.String != "" {
		ed, err := domain.ParseBusinessDate(exitDateStr.String)
		if err != nil {
			return nil, err
		}
		m.ExitDate = &ed
	}
	if exitUserID.Valid {
		uid := exitUserID.Int64
		m.ExitUserID = &uid
	}

	return &m, nil
}

func formatNullableDate(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.Format("2006-01-02")
}
