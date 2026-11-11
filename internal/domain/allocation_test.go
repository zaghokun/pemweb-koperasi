package domain

import (
	"errors"
	"math"
	"testing"
)

type allocRow struct {
	installmentID int64
	component     ComponentType
	amount        int64
}

func rows(t *testing.T, allocs []PaymentAllocation) []allocRow {
	t.Helper()
	out := make([]allocRow, 0, len(allocs))
	for _, a := range allocs {
		if a.InstallmentID == nil {
			t.Fatalf("alokasi %s tanpa InstallmentID", a.ComponentType)
		}
		out = append(out, allocRow{*a.InstallmentID, a.ComponentType, a.Amount})
	}
	return out
}

func assertRows(t *testing.T, got []PaymentAllocation, want []allocRow) {
	t.Helper()
	g := rows(t, got)
	if len(g) != len(want) {
		t.Fatalf("jumlah alokasi = %d, want %d: %+v", len(g), len(want), g)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("alokasi[%d] = %+v, want %+v", i, g[i], want[i])
		}
	}
}

func sumAlloc(allocs []PaymentAllocation) int64 {
	var s int64
	for _, a := range allocs {
		s += a.Amount
	}
	return s
}

// AC-08 / Bagian 6.3: Rp100.000 pada angsuran terlambat 4 hari.
func TestAllocatePaymentPartialExample(t *testing.T) {
	obs := []Obligation{{InstallmentID: 1, InstallmentNumber: 1, Penalty: 8_000, Interest: 60_000, Principal: 500_000}}
	got, err := AllocatePayment(100_000, obs)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, got, []allocRow{
		{1, ComponentPenalty, 8_000},
		{1, ComponentInterest, 60_000},
		{1, ComponentPrincipal, 32_000},
	})
	if sumAlloc(got) != 100_000 {
		t.Fatalf("total alokasi = %d", sumAlloc(got))
	}
}

// AC-09: hanya sebagian denda yang terbayar; jasa dan pokok tidak berkurang.
func TestAllocatePaymentOnlyPartOfPenalty(t *testing.T) {
	obs := []Obligation{{InstallmentID: 1, InstallmentNumber: 1, Penalty: 8_000, Interest: 60_000, Principal: 500_000}}
	got, err := AllocatePayment(5_000, obs)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, got, []allocRow{{1, ComponentPenalty, 5_000}})
}

// AC-10 / KD-04: denda semua angsuran dahulu, lalu jasa, lalu pokok.
func TestAllocatePaymentGlobalPriority(t *testing.T) {
	// Denda 31 hari dan 3 hari seperti AC-07.
	obs := []Obligation{
		{InstallmentID: 10, InstallmentNumber: 1, Penalty: 62_000, Interest: 60_000, Principal: 500_000},
		{InstallmentID: 20, InstallmentNumber: 2, Penalty: 6_000, Interest: 60_000, Principal: 500_000},
	}

	t.Run("uang hanya cukup denda dan sebagian jasa", func(t *testing.T) {
		got, err := AllocatePayment(100_000, obs)
		if err != nil {
			t.Fatal(err)
		}
		assertRows(t, got, []allocRow{
			{10, ComponentPenalty, 62_000},
			{20, ComponentPenalty, 6_000},
			{10, ComponentInterest, 32_000},
		})
	})

	t.Run("pokok baru tersentuh setelah semua jasa", func(t *testing.T) {
		got, err := AllocatePayment(200_000, obs)
		if err != nil {
			t.Fatal(err)
		}
		assertRows(t, got, []allocRow{
			{10, ComponentPenalty, 62_000},
			{20, ComponentPenalty, 6_000},
			{10, ComponentInterest, 60_000},
			{20, ComponentInterest, 60_000},
			{10, ComponentPrincipal, 12_000},
		})
	})
}

// Angsuran tertua dahulu walaupun slice masukan tidak berurutan; masukan tidak diubah.
func TestAllocatePaymentOldestFirstAndInputUntouched(t *testing.T) {
	obs := []Obligation{
		{InstallmentID: 20, InstallmentNumber: 2, Principal: 500_000},
		{InstallmentID: 10, InstallmentNumber: 1, Principal: 500_000},
	}
	snapshot := append([]Obligation(nil), obs...)

	got, err := AllocatePayment(700_000, obs)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, got, []allocRow{
		{10, ComponentPrincipal, 500_000},
		{20, ComponentPrincipal, 200_000},
	})
	for i := range obs {
		if obs[i] != snapshot[i] {
			t.Fatalf("masukan berubah: %+v", obs)
		}
	}
}

// Melunasi seluruh kewajiban tepat; tidak ada alokasi Rp0.
func TestAllocatePaymentExactTotalNoZeroRows(t *testing.T) {
	obs := []Obligation{
		{InstallmentID: 1, InstallmentNumber: 1, Penalty: 0, Interest: 60_000, Principal: 500_000},
		{InstallmentID: 2, InstallmentNumber: 2, Penalty: 2_000, Interest: 0, Principal: 0},
	}
	got, err := AllocatePayment(562_000, obs)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, got, []allocRow{
		{2, ComponentPenalty, 2_000},
		{1, ComponentInterest, 60_000},
		{1, ComponentPrincipal, 500_000},
	})
	for _, a := range got {
		if a.Amount <= 0 {
			t.Fatalf("alokasi nol/negatif: %+v", a)
		}
	}
}

// KD-05 / AC-21: kelebihan ditolak tanpa hasil parsial.
func TestAllocatePaymentRejectsExcess(t *testing.T) {
	obs := []Obligation{{InstallmentID: 1, InstallmentNumber: 1, Interest: 60_000, Principal: 500_000}}
	got, err := AllocatePayment(560_001, obs)
	if !errors.Is(err, ErrPaymentExceedsObligation) {
		t.Fatalf("err = %v, want ErrPaymentExceedsObligation", err)
	}
	if got != nil {
		t.Fatalf("hasil parsial: %+v", got)
	}
}

func TestAllocatePaymentRejectsBadInput(t *testing.T) {
	ok := []Obligation{{InstallmentID: 1, InstallmentNumber: 1, Principal: 100}}
	cases := []struct {
		name   string
		amount int64
		obs    []Obligation
		want   error
	}{
		{"nol", 0, ok, ErrInvalidAmount},
		{"negatif", -1, ok, ErrInvalidAmount},
		{"kewajiban negatif", 10, []Obligation{{InstallmentID: 1, InstallmentNumber: 1, Interest: -5, Principal: 100}}, ErrInvalidAmount},
		{"overflow total", 1, []Obligation{
			{InstallmentID: 1, InstallmentNumber: 1, Principal: math.MaxInt64},
			{InstallmentID: 2, InstallmentNumber: 2, Principal: 1},
		}, ErrOverflow},
		{"tanpa kewajiban", 10, nil, ErrPaymentExceedsObligation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AllocatePayment(tc.amount, tc.obs)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if got != nil {
				t.Fatalf("hasil parsial: %+v", got)
			}
		})
	}
}

// Konservasi nominal pada banyak kombinasi nominal pembayaran.
func TestAllocatePaymentConservesAmount(t *testing.T) {
	obs := []Obligation{
		{InstallmentID: 1, InstallmentNumber: 1, Penalty: 62_000, Interest: 60_000, Principal: 500_000},
		{InstallmentID: 2, InstallmentNumber: 2, Penalty: 6_000, Interest: 60_000, Principal: 500_000},
		{InstallmentID: 3, InstallmentNumber: 3, Interest: 60_000, Principal: 500_000},
	}
	total, err := TotalObligation(obs)
	if err != nil {
		t.Fatal(err)
	}
	for amount := int64(1); amount <= total; amount += 7_919 {
		got, err := AllocatePayment(amount, obs)
		if err != nil {
			t.Fatalf("amount %d: %v", amount, err)
		}
		if s := sumAlloc(got); s != amount {
			t.Fatalf("amount %d: total alokasi %d", amount, s)
		}
	}
	got, err := AllocatePayment(total, obs)
	if err != nil || sumAlloc(got) != total {
		t.Fatalf("lunas total: err=%v sum=%d total=%d", err, sumAlloc(got), total)
	}
}

func TestTotalObligation(t *testing.T) {
	got, err := TotalObligation([]Obligation{
		{InstallmentID: 1, InstallmentNumber: 1, Penalty: 8_000, Interest: 60_000, Principal: 500_000},
	})
	if err != nil || got != 568_000 {
		t.Fatalf("got %d, %v", got, err)
	}
}