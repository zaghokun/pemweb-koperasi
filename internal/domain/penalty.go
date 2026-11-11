package domain

import (
	"time"
)

// CalculatePenalty menghitung total denda dan sisa denda
// berdasarkan snapshot pembayaran dan tanggal perhitungan.
//
// Hasil:
// - total: akumulasi denda sampai cutoff.
// - remaining: denda setelah dikurangi pembayaran.
// - error: kesalahan tanggal atau nominal.
func CalculatePenalty(
	balance InstallmentBalance,
	asOf time.Time,
) (total int64, remaining int64, err error) {

	// Validasi pembayaran denda.
	if balance.PenaltyPaid < 0 {
		return 0, 0, ErrInvalidAmount
	}

	// Normalisasi tanggal jatuh tempo.
	dueDate, err := NormalizeBusinessDate(
		balance.Installment.DueDate,
	)
	if err != nil {
		return 0, 0, err
	}

	// Normalisasi tanggal perhitungan.
	cutoff, err := NormalizeBusinessDate(asOf)
	if err != nil {
		return 0, 0, err
	}

	// Jika angsuran sudah ditutup, gunakan tanggal
	// penutupan apabila lebih awal dari tanggal laporan.
	if balance.Installment.ClosedOn != nil {
		closedDate, err := NormalizeBusinessDate(
			*balance.Installment.ClosedOn,
		)
		if err != nil {
			return 0, 0, err
		}

		if closedDate.Before(cutoff) {
			cutoff = closedDate
		}
	}

	// Hitung keterlambatan berdasarkan hari kalender.
	var overdueDays int64

	if cutoff.After(dueDate) {
		overdueDays = cutoff.Unix()/86400 -
			dueDate.Unix()/86400
	}

	// Hitung total denda dengan pemeriksaan overflow.
	total, err = MultiplyMoney(
		overdueDays,
		DailyPenaltyAmount,
	)
	if err != nil {
		return 0, 0, err
	}

	// Kurangi denda yang sudah dibayar.
	remaining, err = SubtractMoney(
		total,
		balance.PenaltyPaid,
	)
	if err != nil {
		return 0, 0, err
	}

	return total, remaining, nil
}
