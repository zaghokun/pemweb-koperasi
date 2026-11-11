package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestGenerateScheduleExamples(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal int64
		tenor     int
		regular   int64
		last      int64
		interest  int64
	}{
		{"six million over twelve months", 6_000_000, 12, 500_000, 500_000, 60_000},
		{"one million over six months", 1_000_000, 6, 166_666, 166_670, 10_000},
		{"ten million over ten months", 10_000_000, 10, 1_000_000, 1_000_000, 100_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := GenerateSchedule(tc.principal, tc.tenor, scheduleTestDate(t, "2026-01-20"))
			if err != nil || len(rows) != tc.tenor {
				t.Fatalf("GenerateSchedule() returned %d rows, %v", len(rows), err)
			}
			var principalTotal int64
			for i, row := range rows {
				wantPrincipal := tc.regular
				if i == tc.tenor-1 {
					wantPrincipal = tc.last
				}
				if row.PrincipalDue != wantPrincipal || row.InterestDue != tc.interest {
					t.Errorf("row %d amounts = %d + %d; want %d + %d", i+1, row.PrincipalDue, row.InterestDue, wantPrincipal, tc.interest)
				}
				if row.ID != 0 || row.LoanID != 0 || row.InstallmentNumber != i+1 || row.InterestWaived != 0 || row.WaivedOn != nil || row.ClosedOn != nil {
					t.Errorf("row %d not initially unpaid and unbound: %+v", i+1, row)
				}
				principalTotal += row.PrincipalDue
			}
			if principalTotal != tc.principal {
				t.Errorf("principal total = %d; want %d", principalTotal, tc.principal)
			}
		})
	}
}

func TestGenerateScheduleCalendar(t *testing.T) {
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	for _, tc := range []struct {
		name  string
		start time.Time
		first string
		last  string
	}{
		{"January 31", scheduleTestDate(t, "2026-01-31"), "2026-02-10", "2026-07-10"},
		{"December rollover", scheduleTestDate(t, "2026-12-31"), "2027-01-10", "2027-06-10"},
		{"leap year January", scheduleTestDate(t, "2024-01-31"), "2024-02-10", "2024-07-10"},
		{"leap day", scheduleTestDate(t, "2024-02-29"), "2024-03-10", "2024-08-10"},
		{"Jakarta midnight", time.Date(2026, time.March, 1, 0, 15, 3, 4, jakarta), "2026-04-10", "2026-09-10"},
		{"upper year boundary", scheduleTestDate(t, "9999-06-30"), "9999-07-10", "9999-12-10"},
		{"lower year boundary", scheduleTestDate(t, "1000-01-01"), "1000-02-10", "1000-07-10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := GenerateSchedule(6_000_000, 6, tc.start)
			if err != nil || len(rows) != 6 {
				t.Fatalf("GenerateSchedule() returned %d rows, %v", len(rows), err)
			}
			if rows[0].DueDate != scheduleTestDate(t, tc.first) || rows[5].DueDate != scheduleTestDate(t, tc.last) {
				t.Errorf("due dates = %v .. %v; want %s .. %s", rows[0].DueDate, rows[5].DueDate, tc.first, tc.last)
			}
			for i, row := range rows {
				if row.DueDate.Day() != 10 || row.DueDate.Location() != time.UTC || row.DueDate.Hour() != 0 || row.DueDate.Minute() != 0 || row.DueDate.Second() != 0 || row.DueDate.Nanosecond() != 0 {
					t.Errorf("row %d due date is not a normalized day 10: %v", i+1, row.DueDate)
				}
				if i > 0 && row.DueDate != rows[i-1].DueDate.AddDate(0, 1, 0) {
					t.Errorf("row %d skips a calendar month", i+1)
				}
			}
		})
	}
}

func TestGenerateScheduleSmallPrincipal(t *testing.T) {
	start := time.Date(2026, time.March, 1, 0, 15, 0, 0, time.FixedZone("Asia/Jakarta", 7*60*60))
	closed := scheduleTestDate(t, "2026-03-01")
	for _, tc := range []struct {
		principal int64
		interest  int64
	}{
		{1, 0},
		{5, 0},
		{49, 0},
		{50, 1},
		{149, 1},
		{150, 2},
	} {
		rows, err := GenerateSchedule(tc.principal, 6, start)
		if err != nil || len(rows) != 6 {
			t.Fatalf("GenerateSchedule(%d) returned %d rows, %v", tc.principal, len(rows), err)
		}
		var total int64
		for i, row := range rows {
			if row.InterestDue != tc.interest || row.InterestWaived != 0 || row.WaivedOn != nil {
				t.Errorf("principal %d row %d: unexpected interest state %+v", tc.principal, i+1, row)
			}
			if row.PrincipalDue == 0 && row.InterestDue == 0 {
				if row.ClosedOn == nil || *row.ClosedOn != closed {
					t.Errorf("principal %d row %d: zero obligation not closed on disbursement", tc.principal, i+1)
				}
			} else if row.ClosedOn != nil {
				t.Errorf("principal %d row %d: nonzero obligation closed", tc.principal, i+1)
			}
			total += row.PrincipalDue
		}
		if total != tc.principal || rows[5].PrincipalDue == 0 {
			t.Errorf("principal %d: total %d, final principal %d", tc.principal, total, rows[5].PrincipalDue)
		}
	}
}

func TestScheduleValidation(t *testing.T) {
	validDate := scheduleTestDate(t, "2026-01-20")
	for _, tenor := range []int{-1, 0, 5, 25, math.MaxInt} {
		if rows, err := GenerateSchedule(1, tenor, validDate); rows != nil || !errors.Is(err, ErrInvalidTenor) {
			t.Errorf("GenerateSchedule tenor %d = %v, %v", tenor, rows, err)
		}
		if period, err := CurrentPeriod(validDate, tenor, validDate); period != 0 || !errors.Is(err, ErrInvalidTenor) {
			t.Errorf("CurrentPeriod tenor %d = %d, %v", tenor, period, err)
		}
	}
	for _, principal := range []int64{math.MinInt64, -1, 0} {
		if rows, err := GenerateSchedule(principal, 6, validDate); rows != nil || !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("GenerateSchedule principal %d = %v, %v", principal, rows, err)
		}
	}
	for _, invalidDate := range []time.Time{
		{},
		time.Date(999, time.December, 31, 0, 0, 0, 0, time.UTC),
		time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
		scheduleTestDate(t, "9999-07-01"),
		scheduleTestDate(t, "9999-12-31"),
	} {
		if rows, err := GenerateSchedule(1, 6, invalidDate); rows != nil || !errors.Is(err, ErrInvalidDate) {
			t.Errorf("GenerateSchedule date %v = %v, %v", invalidDate, rows, err)
		}
		if period, err := CurrentPeriod(invalidDate, 6, invalidDate); period != 0 || !errors.Is(err, ErrInvalidDate) {
			t.Errorf("CurrentPeriod date %v = %d, %v", invalidDate, period, err)
		}
	}
	for _, asOf := range []time.Time{{}, time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)} {
		if period, err := CurrentPeriod(validDate, 6, asOf); period != 0 || !errors.Is(err, ErrInvalidDate) {
			t.Errorf("CurrentPeriod as-of %v = %d, %v", asOf, period, err)
		}
	}
}

func TestGenerateScheduleOverflowBoundary(t *testing.T) {
	const interest int64 = math.MaxInt64 / 124
	const principal int64 = math.MaxInt64 - interest*24
	start := scheduleTestDate(t, "2026-01-20")
	rows, err := GenerateSchedule(principal, 24, start)
	if err != nil || len(rows) != 24 {
		t.Fatalf("accepted boundary returned %d rows, %v", len(rows), err)
	}
	var total int64
	for _, row := range rows {
		if row.InterestDue != interest {
			t.Errorf("boundary interest = %d; want %d", row.InterestDue, interest)
		}
		total += row.PrincipalDue + row.InterestDue
	}
	if total != math.MaxInt64 {
		t.Errorf("boundary schedule total = %d; want %d", total, int64(math.MaxInt64))
	}
	for _, amount := range []int64{principal + 1, math.MaxInt64} {
		if rows, err := GenerateSchedule(amount, 24, start); rows != nil || !errors.Is(err, ErrOverflow) {
			t.Errorf("overflow principal %d = %v, %v", amount, rows, err)
		}
	}
}

func TestCurrentPeriodBoundaries(t *testing.T) {
	start := scheduleTestDate(t, "2026-01-20")
	for _, tc := range []struct {
		date   string
		period int
	}{
		{"2026-01-20", 1},
		{"2026-01-31", 1},
		{"2026-02-01", 1},
		{"2026-02-10", 1},
		{"2026-02-11", 2},
		{"2026-03-10", 2},
		{"2026-03-11", 3},
		{"2026-07-10", 6},
	} {
		got, err := CurrentPeriod(start, 6, scheduleTestDate(t, tc.date))
		if err != nil || got != tc.period {
			t.Errorf("CurrentPeriod(%s) = %d, %v; want %d", tc.date, got, err, tc.period)
		}
	}
	for _, date := range []string{"2026-01-19", "2026-07-11", "2026-08-10", "2027-01-01"} {
		if got, err := CurrentPeriod(start, 6, scheduleTestDate(t, date)); got != 0 || !errors.Is(err, ErrOutsideSchedule) {
			t.Errorf("CurrentPeriod(%s) = %d, %v; want outside schedule", date, got, err)
		}
	}
}

func TestCurrentPeriodCalendar(t *testing.T) {
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	for _, tc := range []struct {
		name   string
		start  time.Time
		asOf   time.Time
		tenor  int
		period int
	}{
		{"disbursement before day ten", scheduleTestDate(t, "2026-01-01"), scheduleTestDate(t, "2026-01-01"), 6, 1},
		{"same month day eleven", scheduleTestDate(t, "2026-01-01"), scheduleTestDate(t, "2026-01-11"), 6, 1},
		{"January 31 first due", scheduleTestDate(t, "2026-01-31"), scheduleTestDate(t, "2026-02-10"), 6, 1},
		{"December rollover", scheduleTestDate(t, "2026-12-31"), scheduleTestDate(t, "2027-01-11"), 6, 2},
		{"leap day", scheduleTestDate(t, "2024-01-31"), scheduleTestDate(t, "2024-02-29"), 6, 2},
		{"second year", scheduleTestDate(t, "2026-01-20"), scheduleTestDate(t, "2028-01-10"), 24, 24},
		{"Jakarta day eleven", time.Date(2026, time.January, 20, 23, 45, 0, 0, jakarta), time.Date(2026, time.February, 11, 0, 15, 0, 0, jakarta), 6, 2},
		{"due day late time", time.Date(2026, time.January, 20, 23, 45, 0, 0, jakarta), time.Date(2026, time.July, 10, 23, 59, 59, 0, jakarta), 6, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CurrentPeriod(tc.start, tc.tenor, tc.asOf)
			if err != nil || got != tc.period {
				t.Errorf("CurrentPeriod() = %d, %v; want %d", got, err, tc.period)
			}
		})
	}
}

func scheduleTestDate(t *testing.T, text string) time.Time {
	t.Helper()
	date, err := ParseBusinessDate(text)
	if err != nil {
		t.Fatalf("date fixture %q: %v", text, err)
	}
	return date
}
