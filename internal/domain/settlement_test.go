package domain

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

// settlementFixture is Rp6.000.000 / 12 months disbursed 2026-01-20 (Bagian 7.2).
// Installments 1..paid are fully paid and closed on their due date.
func settlementFixture(t *testing.T, paid int) (time.Time, []InstallmentBalance) {
	t.Helper()
	disbursed := scheduleTestDate(t, "2026-01-20")
	rows, err := GenerateSchedule(6_000_000, 12, disbursed)
	if err != nil {
		t.Fatalf("GenerateSchedule() error: %v", err)
	}
	balances := make([]InstallmentBalance, len(rows))
	for i, row := range rows {
		balances[i] = InstallmentBalance{Installment: row}
		if i < paid {
			closed := row.DueDate
			balances[i].Installment.ClosedOn = &closed
			balances[i].PrincipalPaid = row.PrincipalDue
			balances[i].InterestPaid = row.InterestDue
		}
	}
	return disbursed, balances
}

func sumAllocations(allocations []SettlementAllocation) (total int64) {
	for _, a := range allocations {
		total += a.Amount
	}
	return total
}

func TestPreviewEarlySettlementWithoutArrears(t *testing.T) {
	disbursed, balances := settlementFixture(t, 2)
	got, err := PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, "2026-04-05"))
	if err != nil {
		t.Fatalf("PreviewEarlySettlement() error: %v", err)
	}
	if got.CurrentInstallmentNumber != 3 || got.ArrearsAmount != 0 || got.RemainingPrincipal != 5_000_000 ||
		got.CurrentInterest != 60_000 || got.AdministrationFee != 100_000 || got.SettlementAmount != 5_160_000 ||
		got.TotalReceived != 5_160_000 || got.WaivedInterest != 540_000 || len(got.Waivers) != 9 {
		t.Errorf("unexpected breakdown: %+v", got)
	}
	if got.Waivers[0] != (InterestWaiver{4, 60_000}) || got.Waivers[8] != (InterestWaiver{12, 60_000}) {
		t.Errorf("waivers = %+v", got.Waivers)
	}
	if sumAllocations(got.Allocations) != got.TotalReceived {
		t.Errorf("allocations sum %d; want %d", sumAllocations(got.Allocations), got.TotalReceived)
	}
}

func TestPreviewEarlySettlementWithArrears(t *testing.T) {
	disbursed, balances := settlementFixture(t, 2)
	got, err := PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, "2026-04-15"))
	if err != nil {
		t.Fatalf("PreviewEarlySettlement() error: %v", err)
	}
	if got.CurrentInstallmentNumber != 4 || got.ArrearsAmount != 570_000 || got.RemainingPrincipal != 4_500_000 ||
		got.CurrentInterest != 60_000 || got.AdministrationFee != 90_000 || got.SettlementAmount != 4_650_000 ||
		got.TotalReceived != 5_220_000 || got.WaivedInterest != 480_000 || len(got.Waivers) != 8 {
		t.Errorf("unexpected breakdown: %+v", got)
	}
	want := []SettlementAllocation{
		{3, ComponentPenalty, 10_000},
		{3, ComponentInterest, 60_000},
		{4, ComponentInterest, 60_000},
		{3, ComponentPrincipal, 500_000},
		{4, ComponentPrincipal, 500_000},
		{5, ComponentPrincipal, 500_000},
		{6, ComponentPrincipal, 500_000},
		{7, ComponentPrincipal, 500_000},
		{8, ComponentPrincipal, 500_000},
		{9, ComponentPrincipal, 500_000},
		{10, ComponentPrincipal, 500_000},
		{11, ComponentPrincipal, 500_000},
		{12, ComponentPrincipal, 500_000},
		{0, ComponentAdministration, 90_000},
	}
	if !reflect.DeepEqual(got.Allocations, want) {
		t.Errorf("allocations = %+v\nwant %+v", got.Allocations, want)
	}
}

func TestPreviewEarlySettlementPartialPayments(t *testing.T) {
	disbursed, balances := settlementFixture(t, 2)
	// Current period (April, installment 3) interest already half paid.
	balances[2].InterestPaid = 30_000
	got, err := PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, "2026-04-05"))
	if err != nil || got.CurrentInterest != 30_000 || got.TotalReceived != 5_130_000 {
		t.Fatalf("current interest partly paid = %+v, %v", got, err)
	}

	// Arrears with partial penalty, interest and principal payments, plus an
	// already waived amount: only the remainders are received (KD-03, KD-07).
	_, balances = settlementFixture(t, 2)
	balances[2].PenaltyPaid = 4_000
	balances[2].InterestPaid = 20_000
	balances[2].PrincipalPaid = 100_000
	balances[2].Installment.InterestWaived = 10_000
	got, err = PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, "2026-04-15"))
	if err != nil {
		t.Fatalf("PreviewEarlySettlement() error: %v", err)
	}
	if got.ArrearsAmount != 6_000+30_000+400_000 || got.RemainingPrincipal != 4_500_000 || sumAllocations(got.Allocations) != got.TotalReceived {
		t.Errorf("partial arrears = %+v", got)
	}
}

func TestPreviewEarlySettlementPeriodBoundaries(t *testing.T) {
	disbursed, balances := settlementFixture(t, 2)
	for _, tc := range []struct {
		date    string
		current int
		arrears int64
	}{
		{"2026-04-10", 3, 0},       // due date itself is not arrears and has no penalty
		{"2026-04-11", 4, 562_000}, // first overdue day: 1 x Rp2.000
	} {
		got, err := PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, tc.date))
		if err != nil || got.CurrentInstallmentNumber != tc.current || got.ArrearsAmount != tc.arrears {
			t.Errorf("%s = %+v, %v; want current %d", tc.date, got, err, tc.current)
		}
	}

	// First period, disbursement day, and day before the final due date.
	_, fresh := settlementFixture(t, 0)
	got, err := PreviewEarlySettlement(disbursed, fresh, disbursed)
	if err != nil || got.CurrentInstallmentNumber != 1 || got.ArrearsAmount != 0 || got.RemainingPrincipal != 6_000_000 ||
		got.AdministrationFee != 120_000 || got.WaivedInterest != 11*60_000 {
		t.Errorf("disbursement day = %+v, %v", got, err)
	}
	_, nearEnd := settlementFixture(t, 11)
	got, err = PreviewEarlySettlement(disbursed, nearEnd, scheduleTestDate(t, "2027-01-09"))
	if err != nil || got.CurrentInstallmentNumber != 12 || got.WaivedInterest != 0 || got.Waivers != nil ||
		got.RemainingPrincipal != 500_000 || got.AdministrationFee != 10_000 || got.TotalReceived != 570_000 {
		t.Errorf("last period = %+v, %v", got, err)
	}
}

func TestPreviewEarlySettlementAdministrationRounding(t *testing.T) {
	disbursed := scheduleTestDate(t, "2026-01-20")
	for _, tc := range []struct {
		principal int64
		fee       int64
	}{
		{24, 0}, {25, 1}, {74, 1}, {75, 2},
	} {
		rows, err := GenerateSchedule(tc.principal, 6, disbursed)
		if err != nil {
			t.Fatalf("GenerateSchedule(%d) error: %v", tc.principal, err)
		}
		balances := make([]InstallmentBalance, len(rows))
		for i, row := range rows {
			balances[i] = InstallmentBalance{Installment: row}
		}
		got, err := PreviewEarlySettlement(disbursed, balances, disbursed)
		if err != nil || got.AdministrationFee != tc.fee {
			t.Errorf("principal %d fee = %d, %v; want %d", tc.principal, got.AdministrationFee, err, tc.fee)
		}
	}
}

func TestPreviewEarlySettlementUnavailableAndInvalid(t *testing.T) {
	disbursed, balances := settlementFixture(t, 2)
	for _, date := range []string{"2027-01-10", "2027-01-11", "2028-01-01"} {
		if got, err := PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, date)); !errors.Is(err, ErrEarlySettlementUnavailable) || !reflect.DeepEqual(got, EarlySettlement{}) {
			t.Errorf("%s = %+v, %v; want ErrEarlySettlementUnavailable", date, got, err)
		}
	}
	if _, err := PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, "2026-01-19")); !errors.Is(err, ErrOutsideSchedule) {
		t.Errorf("before disbursement error = %v", err)
	}
	if _, err := PreviewEarlySettlement(disbursed, balances, time.Time{}); !errors.Is(err, ErrInvalidDate) {
		t.Errorf("zero settlement date error = %v", err)
	}
	if _, err := PreviewEarlySettlement(disbursed, balances[:5], scheduleTestDate(t, "2026-04-05")); !errors.Is(err, ErrInvalidTenor) {
		t.Errorf("short snapshot error = %v", err)
	}
	if _, err := PreviewEarlySettlement(disbursed.AddDate(0, 1, 0), balances, scheduleTestDate(t, "2026-04-05")); !errors.Is(err, ErrInvalidSchedule) {
		t.Errorf("mismatched disbursement error = %v", err)
	}

	date := scheduleTestDate(t, "2026-04-15")
	for name, mutate := range map[string]func([]InstallmentBalance){
		"wrong number":          func(b []InstallmentBalance) { b[3].Installment.InstallmentNumber = 9 },
		"principal overpaid":    func(b []InstallmentBalance) { b[2].PrincipalPaid = b[2].Installment.PrincipalDue + 1 },
		"interest overpaid":     func(b []InstallmentBalance) { b[3].InterestPaid = b[3].Installment.InterestDue + 1 },
		"waiver plus paid":      func(b []InstallmentBalance) { b[3].Installment.InterestWaived, b[3].InterestPaid = 40_000, 30_000 },
		"penalty overpaid":      func(b []InstallmentBalance) { b[2].PenaltyPaid = 10_001 },
		"negative paid":         func(b []InstallmentBalance) { b[2].PrincipalPaid = -1 },
		"penalty on not due":    func(b []InstallmentBalance) { b[3].PenaltyPaid = 1 },
		"due date out of order": func(b []InstallmentBalance) { b[3].Installment.DueDate = b[1].Installment.DueDate },
	} {
		_, copyBalances := settlementFixture(t, 2)
		mutate(copyBalances)
		if got, err := PreviewEarlySettlement(disbursed, copyBalances, date); err == nil || !reflect.DeepEqual(got, EarlySettlement{}) {
			t.Errorf("%s = %+v, %v; want error", name, got, err)
		}
	}
	if _, err := PreviewEarlySettlement(disbursed, fullyPaid(t), scheduleTestDate(t, "2026-04-05")); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("nothing to receive error = %v", err)
	}
}

func fullyPaid(t *testing.T) []InstallmentBalance {
	_, balances := settlementFixture(t, 12)
	return balances
}

func TestPreviewEarlySettlementOverflow(t *testing.T) {
	disbursed := scheduleTestDate(t, "2026-01-20")
	rows, err := GenerateSchedule(6, 6, disbursed)
	if err != nil {
		t.Fatalf("GenerateSchedule() error: %v", err)
	}
	balances := make([]InstallmentBalance, len(rows))
	for i, row := range rows {
		balances[i] = InstallmentBalance{Installment: row}
	}
	balances[1].Installment.PrincipalDue = math.MaxInt64
	balances[2].Installment.PrincipalDue = 1
	if got, err := PreviewEarlySettlement(disbursed, balances, disbursed); !errors.Is(err, ErrOverflow) || !reflect.DeepEqual(got, EarlySettlement{}) {
		t.Errorf("overflow = %+v, %v", got, err)
	}
}

func TestPreviewEarlySettlementDoesNotMutateInput(t *testing.T) {
	disbursed, balances := settlementFixture(t, 2)
	snapshot := append([]InstallmentBalance(nil), balances...)
	for i := range snapshot {
		if balances[i].Installment.ClosedOn != nil {
			closed := *balances[i].Installment.ClosedOn
			snapshot[i].Installment.ClosedOn = &closed
		}
	}
	if _, err := PreviewEarlySettlement(disbursed, balances, scheduleTestDate(t, "2026-04-15")); err != nil {
		t.Fatalf("PreviewEarlySettlement() error: %v", err)
	}
	if !reflect.DeepEqual(snapshot, balances) {
		t.Error("input snapshot was modified")
	}
}
