package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidTenor    = errors.New("tenor must be between 6 and 24 months")
	ErrOutsideSchedule = errors.New("date is outside the loan schedule")
)

// GenerateSchedule leaves persistence IDs unassigned and uses calendar dates.
func GenerateSchedule(principal int64, tenorMonths int, disbursementDate time.Time) ([]Installment, error) {
	if principal <= 0 {
		return nil, ErrInvalidAmount
	}
	disbursementDate, _, err := scheduleBounds(disbursementDate, tenorMonths)
	if err != nil {
		return nil, err
	}
	interest, err := RoundHalfUp(principal, MonthlyInterestDivisor)
	if err != nil {
		return nil, err
	}
	totalInterest, err := MultiplyMoney(interest, int64(tenorMonths))
	if err != nil {
		return nil, err
	}
	if _, err := AddMoney(principal, totalInterest); err != nil {
		return nil, err
	}
	regularPrincipal := principal / int64(tenorMonths)
	lastPrincipal := regularPrincipal + principal%int64(tenorMonths)
	year, month, _ := disbursementDate.Date()

	schedule := make([]Installment, tenorMonths)
	for i := range schedule {
		row := &schedule[i]
		row.InstallmentNumber = i + 1
		row.DueDate = time.Date(year, month+time.Month(i+1), 10, 0, 0, 0, 0, time.UTC)
		row.PrincipalDue = regularPrincipal
		if i == tenorMonths-1 {
			row.PrincipalDue = lastPrincipal
		}
		row.InterestDue = interest
		if row.PrincipalDue == 0 && row.InterestDue == 0 {
			closedOn := disbursementDate
			row.ClosedOn = &closedOn
		}
	}
	return schedule, nil
}

// CurrentPeriod includes each due date in its ending period, never after maturity.
func CurrentPeriod(disbursementDate time.Time, tenorMonths int, asOf time.Time) (int, error) {
	disbursementDate, finalDue, err := scheduleBounds(disbursementDate, tenorMonths)
	if err != nil {
		return 0, err
	}
	asOf, err = NormalizeBusinessDate(asOf)
	if err != nil {
		return 0, err
	}
	if asOf.Before(disbursementDate) || asOf.After(finalDue) {
		return 0, ErrOutsideSchedule
	}
	period := (asOf.Year()-disbursementDate.Year())*12 + int(asOf.Month()) - int(disbursementDate.Month())
	if asOf.Day() > 10 {
		period++
	}
	if period < 1 {
		period = 1
	}
	return period, nil
}

func scheduleBounds(disbursementDate time.Time, tenorMonths int) (time.Time, time.Time, error) {
	if tenorMonths < MinTenorMonths || tenorMonths > MaxTenorMonths {
		return time.Time{}, time.Time{}, ErrInvalidTenor
	}
	disbursementDate, err := NormalizeBusinessDate(disbursementDate)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	finalDue, err := NormalizeBusinessDate(time.Date(disbursementDate.Year(), disbursementDate.Month()+time.Month(tenorMonths), 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return disbursementDate, finalDue, nil
}
