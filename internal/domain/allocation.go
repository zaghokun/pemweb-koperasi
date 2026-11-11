package domain

import (
	"errors"
	"math"
	"sort"
)

// ErrPaymentExceedsObligation dikembalikan ketika uang yang dibayarkan melebihi
// seluruh kewajiban yang boleh dibayar (KD-05): kelebihan ditolak, bukan hilang
// atau otomatis menjadi simpanan.
var ErrPaymentExceedsObligation = errors.New("pembayaran melebihi kewajiban yang boleh dibayar")

// Obligation adalah snapshot sisa kewajiban satu angsuran pada tanggal
// pembayaran. Penalty harus sudah dihitung untuk tanggal tersebut (penalty.go);
// fungsi di file ini tidak menghitung denda, jasa, atau tanggal sendiri.
// Semua nominal adalah SISA yang belum dibayar, tidak negatif.
type Obligation struct {
	InstallmentID     int64
	InstallmentNumber int
	Penalty           int64
	Interest          int64
	Principal         int64
}

// allocationOrder adalah prioritas global komponen (BR-12, KD-04).
var allocationOrder = [...]ComponentType{
	ComponentPenalty,
	ComponentInterest,
	ComponentPrincipal,
}

func (o Obligation) amountOf(c ComponentType) int64 {
	switch c {
	case ComponentPenalty:
		return o.Penalty
	case ComponentInterest:
		return o.Interest
	default:
		return o.Principal
	}
}

// TotalObligation menjumlahkan seluruh sisa denda, jasa, dan pokok. Berguna
// untuk batas maksimum pembayaran biasa. Nominal negatif ditolak dengan
// ErrInvalidAmount dan penjumlahan yang melewati int64 dengan ErrOverflow.
func TotalObligation(obligations []Obligation) (int64, error) {
	var total int64
	for _, o := range obligations {
		for _, c := range allocationOrder {
			v := o.amountOf(c)
			if v < 0 {
				return 0, ErrInvalidAmount
			}
			if total > math.MaxInt64-v {
				return 0, ErrOverflow
			}
			total += v
		}
	}
	return total, nil
}

// AllocatePayment membagi uang yang diterima ke komponen kewajiban dengan
// prioritas global: semua denda dahulu, lalu semua jasa, lalu semua pokok.
// Di dalam tiap komponen, angsuran bernomor terkecil (tertua) dahulu.
//
// Aturan hasil:
//   - jumlah seluruh alokasi tepat sama dengan amount;
//   - tidak ada alokasi bernilai nol;
//   - slice obligations tidak diubah;
//   - hasil belum disimpan: ID dan PaymentID bernilai nol.
//
// Error: ErrInvalidAmount (amount <= 0 atau kewajiban negatif),
// ErrOverflow, dan ErrPaymentExceedsObligation bila amount melebihi total
// kewajiban. Pada error tidak ada hasil parsial.
func AllocatePayment(amount int64, obligations []Obligation) ([]PaymentAllocation, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	total, err := TotalObligation(obligations)
	if err != nil {
		return nil, err
	}
	if amount > total {
		return nil, ErrPaymentExceedsObligation
	}

	ordered := make([]Obligation, len(obligations))
	copy(ordered, obligations)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].InstallmentNumber < ordered[j].InstallmentNumber
	})

	remaining := amount
	var result []PaymentAllocation
	for _, component := range allocationOrder {
		for _, o := range ordered {
			if remaining == 0 {
				return result, nil
			}
			due := o.amountOf(component)
			if due == 0 {
				continue
			}
			pay := due
			if remaining < pay {
				pay = remaining
			}
			id := o.InstallmentID
			result = append(result, PaymentAllocation{
				InstallmentID: &id,
				ComponentType: component,
				Amount:        pay,
			})
			remaining -= pay
		}
	}
	return result, nil
}