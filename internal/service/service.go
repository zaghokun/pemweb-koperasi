package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("username atau password salah")
	ErrSessionExpired     = errors.New("sesi telah kedaluwarsa")
	ErrUserInactive       = errors.New("akun pengurus tidak aktif")
	ErrUnauthorized       = errors.New("akses ditolak")
	ErrInvalidCSRF        = errors.New("token CSRF tidak sah")
	ErrFutureDate         = errors.New("tanggal transaksi tidak boleh setelah hari ini")
	ErrBeforeJoinDate     = errors.New("tanggal transaksi tidak boleh sebelum anggota bergabung")
	ErrBackdated          = errors.New("tanggal transaksi tidak boleh sebelum transaksi keuangan terakhir anggota")
)

var jakartaLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("Asia/Jakarta", 7*60*60)
	}
	return loc
}()

func BusinessToday() time.Time {
	date, _ := domain.NormalizeBusinessDate(time.Now().In(jakartaLocation))
	return date
}

func validateMemberDate(ctx context.Context, tx *sql.Tx, repo *repository.MemberRepository, member *domain.Member, date time.Time) error {
	if date.After(BusinessToday()) {
		return ErrFutureDate
	}
	if date.Before(member.JoinDate) {
		return ErrBeforeJoinDate
	}
	latest, err := repo.LastFinancialDate(ctx, tx, member.ID)
	if err != nil {
		return err
	}
	if latest != nil && date.Before(*latest) {
		return ErrBackdated
	}
	return nil
}

func lockLoanAndMember(ctx context.Context, tx *sql.Tx, loans *repository.LoanRepository, members *repository.MemberRepository, loanID int64) (*domain.Loan, *domain.Member, error) {
	loan, err := loans.GetByID(ctx, tx, loanID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil, ErrLoanNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	member, err := members.GetByIDForUpdate(ctx, tx, loan.MemberID)
	if err != nil {
		return nil, nil, err
	}
	loan, err = loans.GetByIDForUpdate(ctx, tx, loanID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil, ErrLoanNotFound
	}
	return loan, member, err
}

type UserRepo interface {
	GetByID(ctx context.Context, db repository.DBTX, id int64) (*domain.User, error)
	GetByUsername(ctx context.Context, db repository.DBTX, username string) (*domain.User, error)
	Create(ctx context.Context, db repository.DBTX, user *domain.User) error
}

type SessionRepo interface {
	Create(ctx context.Context, db repository.DBTX, s *domain.Session) error
	GetByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) (*domain.Session, error)
	DeleteByTokenHash(ctx context.Context, db repository.DBTX, tokenHash [32]byte) error
	DeleteExpired(ctx context.Context, db repository.DBTX, now time.Time) error
}
