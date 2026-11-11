package domain

import (
	"errors"
	"math"
	"strconv"
)

var (
	ErrInvalidAmount  = errors.New("amount must be a nonnegative whole rupiah value; parsed amounts must be positive")
	ErrOverflow       = errors.New("amount exceeds int64 capacity")
	ErrInvalidDivisor = errors.New("divisor must be positive")
)

func ParseRupiah(value string) (int64, error) {
	if value == "" {
		return 0, ErrInvalidAmount
	}
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return 0, ErrInvalidAmount
		}
	}
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, ErrOverflow
	}
	if amount == 0 {
		return 0, ErrInvalidAmount
	}
	return amount, nil
}

func AddMoney(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, ErrInvalidAmount
	}
	if a > math.MaxInt64-b {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func SubtractMoney(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a < b {
		return 0, ErrInvalidAmount
	}
	return a - b, nil
}

func MultiplyMoney(amount, factor int64) (int64, error) {
	if amount < 0 || factor < 0 {
		return 0, ErrInvalidAmount
	}
	if factor != 0 && amount > math.MaxInt64/factor {
		return 0, ErrOverflow
	}
	return amount * factor, nil
}

func RoundHalfUp(amount, divisor int64) (int64, error) {
	if amount < 0 {
		return 0, ErrInvalidAmount
	}
	if divisor <= 0 {
		return 0, ErrInvalidDivisor
	}
	quotient, remainder := amount/divisor, amount%divisor
	if remainder >= divisor/2+divisor%2 {
		quotient++
	}
	return quotient, nil
}
