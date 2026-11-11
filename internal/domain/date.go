package domain

import (
	"errors"
	"time"
)

var ErrInvalidDate = errors.New("business date must be within MySQL DATE range 1000-01-01 to 9999-12-31")

// NormalizeBusinessDate preserves the supplied calendar fields, not the instant.
// UTC midnight is a date carrier; convert real instants to Jakarta before calling.
func NormalizeBusinessDate(value time.Time) (time.Time, error) {
	year, month, day := value.Date()
	if year < 1000 || year > 9999 {
		return time.Time{}, ErrInvalidDate
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

func ParseBusinessDate(value string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, ErrInvalidDate
	}
	return NormalizeBusinessDate(date)
}
