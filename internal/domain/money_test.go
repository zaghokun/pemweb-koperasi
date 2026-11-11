package domain

import (
	"errors"
	"math"
	"testing"
)

func TestParseRupiah(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int64
		err  error
	}{
		{text: "1", want: 1},
		{text: "000500000", want: 500_000},
		{text: "9223372036854775807", want: math.MaxInt64},
		{text: "", err: ErrInvalidAmount},
		{text: "0", err: ErrInvalidAmount},
		{text: "-1", err: ErrInvalidAmount},
		{text: "+1", err: ErrInvalidAmount},
		{text: "1,000", err: ErrInvalidAmount},
		{text: "1.000", err: ErrInvalidAmount},
		{text: " 100 ", err: ErrInvalidAmount},
		{text: "1 000", err: ErrInvalidAmount},
		{text: "100\n", err: ErrInvalidAmount},
		{text: "1.5", err: ErrInvalidAmount},
		{text: "1e3", err: ErrInvalidAmount},
		{text: "Rp100", err: ErrInvalidAmount},
		{text: "１２３", err: ErrInvalidAmount},
		{text: "9223372036854775808", err: ErrOverflow},
		{text: "9223372036854775808x", err: ErrInvalidAmount},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, err := ParseRupiah(tc.text)
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Fatalf("ParseRupiah(%q) = %d, %v; want %d, %v", tc.text, got, err, tc.want, tc.err)
			}
		})
	}
}

func TestMoneyArithmetic(t *testing.T) {
	for _, operation := range []struct {
		name  string
		apply func(int64, int64) (int64, error)
		cases []struct {
			a, b int64
			want int64
			err  error
		}
	}{
		{
			name:  "add",
			apply: AddMoney,
			cases: []struct {
				a, b int64
				want int64
				err  error
			}{
				{a: 500_000, b: 60_000, want: 560_000},
				{a: 0, b: 0, want: 0},
				{a: math.MaxInt64 - 60_000, b: 60_000, want: math.MaxInt64},
				{a: math.MaxInt64 - 60_000, b: 60_001, err: ErrOverflow},
				{a: -1, b: 1, err: ErrInvalidAmount},
				{a: 1, b: math.MinInt64, err: ErrInvalidAmount},
			},
		},
		{
			name:  "subtract",
			apply: SubtractMoney,
			cases: []struct {
				a, b int64
				want int64
				err  error
			}{
				{a: 100_000, b: 60_000, want: 40_000},
				{a: math.MaxInt64, b: 0, want: math.MaxInt64},
				{a: math.MaxInt64, b: math.MaxInt64, want: 0},
				{a: 0, b: 1, err: ErrInvalidAmount},
				{a: -1, b: 0, err: ErrInvalidAmount},
				{a: 0, b: math.MinInt64, err: ErrInvalidAmount},
			},
		},
		{
			name:  "multiply",
			apply: MultiplyMoney,
			cases: []struct {
				a, b int64
				want int64
				err  error
			}{
				{a: 500_000, b: 3, want: 1_500_000},
				{a: 0, b: math.MaxInt64, want: 0},
				{a: math.MaxInt64, b: 0, want: 0},
				{a: math.MaxInt64 / 7, b: 7, want: math.MaxInt64},
				{a: math.MaxInt64/7 + 1, b: 7, err: ErrOverflow},
				{a: math.MinInt64, b: 0, err: ErrInvalidAmount},
				{a: 0, b: -1, err: ErrInvalidAmount},
			},
		},
	} {
		t.Run(operation.name, func(t *testing.T) {
			for _, tc := range operation.cases {
				got, err := operation.apply(tc.a, tc.b)
				if got != tc.want || !errors.Is(err, tc.err) {
					t.Errorf("%s(%d, %d) = %d, %v; want %d, %v", operation.name, tc.a, tc.b, got, err, tc.want, tc.err)
				}
			}
		})
	}
}

func TestRoundHalfUp(t *testing.T) {
	for _, tc := range []struct {
		amount, divisor int64
		want            int64
		err             error
	}{
		{amount: 6_000_000, divisor: 100, want: 60_000},
		{amount: 49, divisor: 100, want: 0},
		{amount: 50, divisor: 100, want: 1},
		{amount: 150, divisor: 100, want: 2},
		{amount: 7, divisor: 3, want: 2},
		{amount: 8, divisor: 3, want: 3},
		{amount: 0, divisor: 100, want: 0},
		{amount: math.MaxInt64, divisor: 1, want: math.MaxInt64},
		{amount: math.MaxInt64, divisor: 2, want: 4_611_686_018_427_387_904},
		{amount: math.MaxInt64 / 2, divisor: math.MaxInt64, want: 0},
		{amount: math.MaxInt64/2 + 1, divisor: math.MaxInt64, want: 1},
		{amount: math.MaxInt64, divisor: math.MaxInt64, want: 1},
		{amount: -1, divisor: 100, err: ErrInvalidAmount},
		{amount: 100, divisor: 0, err: ErrInvalidDivisor},
		{amount: 100, divisor: math.MinInt64, err: ErrInvalidDivisor},
	} {
		got, err := RoundHalfUp(tc.amount, tc.divisor)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("RoundHalfUp(%d, %d) = %d, %v; want %d, %v", tc.amount, tc.divisor, got, err, tc.want, tc.err)
		}
	}
}
