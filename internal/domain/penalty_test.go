package domain

import (
	"errors"
	"testing"
	"time"
)

func penaltyTestDate(t *testing.T, value string) time.Time {
	t.Helper()

	date, err := ParseBusinessDate(value)
	if err != nil {
		t.Fatalf("ParseBusinessDate() error: %v", err)
	}

	return date
}

func TestCalculatePenalty(t *testing.T) {
	tests := []struct {
		name          string
		dueDate       string
		asOf          string
		closedOn      *string
		penaltyPaid   int64
		wantTotal     int64
		wantRemaining int64
	}{
		{
			name:          "on due date",
			dueDate:       "2026-02-10",
			asOf:          "2026-02-10",
			wantTotal:     0,
			wantRemaining: 0,
		},
		{
			name:          "one day late",
			dueDate:       "2026-02-10",
			asOf:          "2026-02-11",
			wantTotal:     2000,
			wantRemaining: 2000,
		},
		{
			name:          "four days late",
			dueDate:       "2026-02-10",
			asOf:          "2026-02-14",
			wantTotal:     8000,
			wantRemaining: 8000,
		},
		{
			name:          "partial penalty payment",
			dueDate:       "2026-02-10",
			asOf:          "2026-02-14",
			penaltyPaid:   3000,
			wantTotal:     8000,
			wantRemaining: 5000,
		},
		{
			name:          "closed installment freezes penalty",
			dueDate:       "2026-02-10",
			asOf:          "2026-02-15",
			closedOn:      stringPointer("2026-02-13"),
			wantTotal:     6000,
			wantRemaining: 6000,
		},
		{
			name:          "historical date before closure",
			dueDate:       "2026-02-10",
			asOf:          "2026-02-12",
			closedOn:      stringPointer("2026-02-13"),
			wantTotal:     4000,
			wantRemaining: 4000,
		},
		{
			name:          "closed before due date",
			dueDate:       "2026-02-10",
			asOf:          "2026-02-15",
			closedOn:      stringPointer("2026-02-09"),
			wantTotal:     0,
			wantRemaining: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installment := Installment{
				DueDate: penaltyTestDate(t, tt.dueDate),
			}

			if tt.closedOn != nil {
				closed := penaltyTestDate(t, *tt.closedOn)
				installment.ClosedOn = &closed
			}

			balance := InstallmentBalance{
				Installment: installment,
				PenaltyPaid: tt.penaltyPaid,
			}

			total, remaining, err := CalculatePenalty(
				balance,
				penaltyTestDate(t, tt.asOf),
			)

			if err != nil {
				t.Fatalf("CalculatePenalty() error: %v", err)
			}

			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}

			if remaining != tt.wantRemaining {
				t.Errorf(
					"remaining = %d, want %d",
					remaining,
					tt.wantRemaining,
				)
			}
		})
	}
}

func TestCalculatePenaltyInvalidPayment(t *testing.T) {
	balance := InstallmentBalance{
		Installment: Installment{
			DueDate: penaltyTestDate(t, "2026-02-10"),
		},
		PenaltyPaid: -1,
	}

	_, _, err := CalculatePenalty(
		balance,
		penaltyTestDate(t, "2026-02-15"),
	)

	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("got error %v, want ErrInvalidAmount", err)
	}
}

func TestCalculatePenaltyPaymentExceedsTotal(t *testing.T) {
	balance := InstallmentBalance{
		Installment: Installment{
			DueDate: penaltyTestDate(t, "2026-02-10"),
		},
		PenaltyPaid: 12000,
	}

	_, _, err := CalculatePenalty(
		balance,
		penaltyTestDate(t, "2026-02-14"),
	)

	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("got error %v, want ErrInvalidAmount", err)
	}
}

func stringPointer(value string) *string {
	return &value
}
