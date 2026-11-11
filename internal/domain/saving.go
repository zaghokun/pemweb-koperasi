package domain

import "time"

type SavingType string
type SavingTransactionType string

const (
	SavingPrincipal SavingType = "POKOK"
	SavingMandatory SavingType = "WAJIB"
	SavingVoluntary SavingType = "SUKARELA"

	SavingDeposit    SavingTransactionType = "SETOR"
	SavingWithdrawal SavingTransactionType = "TARIK"
	SavingExitRefund SavingTransactionType = "PENGEMBALIAN_KELUAR"

	PrincipalSavingAmount int64 = 500_000
	MandatorySavingAmount int64 = 100_000
)

type Saving struct {
	ID              int64
	MemberID        int64
	UserID          int64
	SavingType      SavingType
	TransactionType SavingTransactionType
	Amount          int64
	PeriodMonth     *time.Time
	TransactionDate time.Time
	RequestKey      string
	Description     *string
	CreatedAt       time.Time
}
