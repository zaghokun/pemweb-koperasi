package domain

import (
	"errors"
	"time"
)

var (
	ErrEarlySettlementUnavailable = errors.New("early settlement requires a date before the final due date")
	ErrInvalidSchedule            = errors.New("installment snapshot does not match the loan schedule")
)

// SettlementAllocation is one component of the single early-settlement receipt.
// InstallmentNumber is 0 only for ADMIN; the service maps numbers to installment IDs.
type SettlementAllocation struct {
	InstallmentNumber int
	Component         ComponentType
	Amount            int64
}

// InterestWaiver is unpaid future interest to record on the installment as
// InterestWaived. It is not a paid component and never becomes income.
type InterestWaiver struct {
	InstallmentNumber int
	Amount            int64
}

// EarlySettlement mirrors the loan_settlements snapshot (Bagian 7). The service
// closes every installment and the loan on the settlement date in the same transaction.
type EarlySettlement struct {
	CurrentInstallmentNumber int
	ArrearsAmount            int64 // stage 1: all installments with due date before the settlement date
	RemainingPrincipal       int64 // P_sisa after stage 1
	CurrentInterest          int64 // J_berjalan: unpaid interest of the running period
	AdministrationFee        int64 // A = 2% of RemainingPrincipal, half-up
	SettlementAmount         int64 // L = P_sisa + J_berjalan + A
	TotalReceived            int64 // T = ArrearsAmount + SettlementAmount
	WaivedInterest           int64
	Allocations              []SettlementAllocation // ordered DENDA, JASA, POKOK, ADMIN; oldest first; sums to TotalReceived
	Waivers                  []InterestWaiver
}

// PreviewEarlySettlement computes the early-settlement breakdown without side effects.
//
// balances must hold every installment of the loan in order, aggregated up to
// settlementDate. An installment paid in full must already carry ClosedOn (KD-03),
// otherwise penalty keeps accruing. At or after the final due date it returns
// ErrEarlySettlementUnavailable: pay the remaining obligations as a regular payment
// without administration fee (KD-09).
func PreviewEarlySettlement(disbursementDate time.Time, balances []InstallmentBalance, settlementDate time.Time) (EarlySettlement, error) {
	var zero EarlySettlement
	tenor := len(balances)
	if tenor < MinTenorMonths || tenor > MaxTenorMonths {
		return zero, ErrInvalidTenor
	}
	settlementDate, err := NormalizeBusinessDate(settlementDate)
	if err != nil {
		return zero, err
	}
	finalDue, err := NormalizeBusinessDate(balances[tenor-1].Installment.DueDate)
	if err != nil {
		return zero, err
	}
	if !settlementDate.Before(finalDue) {
		return zero, ErrEarlySettlementUnavailable
	}
	period, err := CurrentPeriod(disbursementDate, tenor, settlementDate)
	if err != nil {
		return zero, err
	}
	current := period - 1

	var penaltyRem, interestRem, principalRem = make([]int64, tenor), make([]int64, tenor), make([]int64, tenor)
	for i, balance := range balances {
		if balance.Installment.InstallmentNumber != i+1 {
			return zero, ErrInvalidSchedule
		}
		due, err := NormalizeBusinessDate(balance.Installment.DueDate)
		if err != nil {
			return zero, err
		}
		// The running installment is the first one not yet overdue.
		if (i < current) != due.Before(settlementDate) {
			return zero, ErrInvalidSchedule
		}
		if _, penaltyRem[i], err = CalculatePenalty(balance, settlementDate); err != nil {
			return zero, err
		}
		if interestRem[i], err = SubtractMoney(balance.Installment.InterestDue, balance.Installment.InterestWaived); err != nil {
			return zero, err
		}
		if interestRem[i], err = SubtractMoney(interestRem[i], balance.InterestPaid); err != nil {
			return zero, err
		}
		if principalRem[i], err = SubtractMoney(balance.Installment.PrincipalDue, balance.PrincipalPaid); err != nil {
			return zero, err
		}
	}

	result := EarlySettlement{CurrentInstallmentNumber: period}
	var penalties, interests, principals []SettlementAllocation
	add := func(list *[]SettlementAllocation, number int, component ComponentType, amount int64) {
		if amount > 0 {
			*list = append(*list, SettlementAllocation{InstallmentNumber: number, Component: component, Amount: amount})
		}
	}
	for i := range balances {
		number := i + 1
		add(&penalties, number, ComponentPenalty, penaltyRem[i])
		add(&principals, number, ComponentPrincipal, principalRem[i])
		switch {
		case i < current:
			add(&interests, number, ComponentInterest, interestRem[i])
			if result.ArrearsAmount, err = addAll(result.ArrearsAmount, penaltyRem[i], interestRem[i], principalRem[i]); err != nil {
				return zero, err
			}
		case i == current:
			add(&interests, number, ComponentInterest, interestRem[i])
			result.CurrentInterest = interestRem[i]
		default:
			if interestRem[i] > 0 {
				result.Waivers = append(result.Waivers, InterestWaiver{InstallmentNumber: number, Amount: interestRem[i]})
				if result.WaivedInterest, err = AddMoney(result.WaivedInterest, interestRem[i]); err != nil {
					return zero, err
				}
			}
		}
		if i >= current {
			if result.RemainingPrincipal, err = AddMoney(result.RemainingPrincipal, principalRem[i]); err != nil {
				return zero, err
			}
		}
	}

	if result.AdministrationFee, err = RoundHalfUp(result.RemainingPrincipal, AdministrationDivisor); err != nil {
		return zero, err
	}
	if result.SettlementAmount, err = addAll(result.RemainingPrincipal, result.CurrentInterest, result.AdministrationFee); err != nil {
		return zero, err
	}
	if result.TotalReceived, err = AddMoney(result.ArrearsAmount, result.SettlementAmount); err != nil {
		return zero, err
	}
	if result.TotalReceived == 0 {
		return zero, ErrInvalidAmount // nothing left to receive; a payment cannot be zero
	}

	for _, list := range [][]SettlementAllocation{penalties, interests, principals} {
		result.Allocations = append(result.Allocations, list...)
	}
	if result.AdministrationFee > 0 {
		result.Allocations = append(result.Allocations, SettlementAllocation{Component: ComponentAdministration, Amount: result.AdministrationFee})
	}
	return result, nil
}

func addAll(values ...int64) (int64, error) {
	var total int64
	for _, value := range values {
		var err error
		if total, err = AddMoney(total, value); err != nil {
			return 0, err
		}
	}
	return total, nil
}
