package domain

import "time"

type LoanStatus string

const (
	LoanPending  LoanStatus = "PENDING"
	LoanApproved LoanStatus = "APPROVED"
	LoanActive   LoanStatus = "ACTIVE"
	LoanSettled  LoanStatus = "SETTLED"
	LoanRejected LoanStatus = "REJECTED"

	MinTenorMonths = 6
	MaxTenorMonths = 24

	LoanLimitMultiplier int64 = 3
	// Fixed divisors encode the agreed 1% interest and 2% administration rates.
	MonthlyInterestDivisor int64 = 100
	AdministrationDivisor  int64 = 50
)

type Loan struct {
	ID               int64
	MemberID         int64
	Principal        int64
	TenorMonths      int
	ApplicationDate  time.Time
	Status           LoanStatus
	CreatedBy        int64
	ApprovedBy       *int64
	DisbursedBy      *int64
	RejectedBy       *int64
	DisbursementDate *time.Time
	SettledOn        *time.Time
	RejectionReason  *string
	CreatedAt        time.Time
}

type Installment struct {
	ID                int64
	LoanID            int64
	InstallmentNumber int
	DueDate           time.Time
	PrincipalDue      int64
	InterestDue       int64
	InterestWaived    int64
	WaivedOn          *time.Time
	ClosedOn          *time.Time
}

// InstallmentBalance contains payments aggregated up to the caller's as-of date.
// These totals are snapshots, not additional persisted balance columns.
type InstallmentBalance struct {
	Installment   Installment
	PrincipalPaid int64
	InterestPaid  int64
	PenaltyPaid   int64
}
