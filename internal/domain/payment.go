package domain

import "time"

type PaymentType string
type ComponentType string

const (
	PaymentRegular         PaymentType = "REGULAR"
	PaymentEarlySettlement PaymentType = "EARLY_SETTLEMENT"

	ComponentPenalty        ComponentType = "DENDA"
	ComponentInterest       ComponentType = "JASA"
	ComponentPrincipal      ComponentType = "POKOK"
	ComponentAdministration ComponentType = "ADMIN"

	DailyPenaltyAmount int64 = 2_000
)

type Payment struct {
	ID          int64
	LoanID      int64
	UserID      int64
	PaymentType PaymentType
	PaymentDate time.Time
	Amount      int64
	RequestKey  string
	CreatedAt   time.Time
}

type PaymentAllocation struct {
	ID            int64
	PaymentID     int64
	InstallmentID *int64
	ComponentType ComponentType
	Amount        int64
}
