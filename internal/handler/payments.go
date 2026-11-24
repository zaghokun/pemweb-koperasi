package handler

import (
	"errors"
	"net/http"
	"strconv"

	"koperasi/internal/domain"
	"koperasi/internal/middleware"
	"koperasi/internal/service"
)

// ShowNewPayment menampilkan form input pembayaran beserta pratinjau kewajiban dan denda terakumulasi.
func (h *Handler) ShowNewPayment(w http.ResponseWriter, r *http.Request) {
	loanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	dateStr := r.URL.Query().Get("payment_date")
	if dateStr == "" {
		dateStr = service.BusinessToday().Format("2006-01-02")
	}

	payDate, err := domain.ParseBusinessDate(dateStr)
	if err != nil {
		payDate = service.BusinessToday()
	}

	preview, err := h.paymentSvc.Preview(r.Context(), loanID, payDate)
	if err != nil {
		if errors.Is(err, service.ErrLoanNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Gagal menghitung kewajiban pembayaran: "+err.Error(), http.StatusBadRequest)
		return
	}

	reqKey := generateRequestKey()

	h.Render(w, r, "payments/form.html", map[string]any{
		"Preview":     preview,
		"PaymentDate": payDate.Format("2006-01-02"),
		"RequestKey":  reqKey,
	})
}

// CreatePayment memproses pembayaran pinjaman biasa (penuh atau sebagian) dengan alokasi otomatis denda -> jasa -> pokok.
func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	loanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	amountStr := r.FormValue("amount")
	amount, err := domain.ParseRupiah(amountStr)
	if err != nil {
		h.renderPaymentError(w, r, loanID, "Nominal pembayaran tidak valid (gunakan digit rupiah bulat positif)")
		return
	}

	payDateStr := r.FormValue("payment_date")
	payDate, err := domain.ParseBusinessDate(payDateStr)
	if err != nil {
		h.renderPaymentError(w, r, loanID, "Format tanggal pembayaran tidak valid (gunakan YYYY-MM-DD)")
		return
	}

	reqKey := r.FormValue("request_key")

	result, err := h.paymentSvc.Pay(r.Context(), service.PayInput{
		LoanID:      loanID,
		UserID:      user.ID,
		Amount:      amount,
		PaymentDate: payDate,
		RequestKey:  reqKey,
	})
	if err != nil {
		errMsg := "Gagal memproses pembayaran: " + err.Error()
		if errors.Is(err, domain.ErrPaymentExceedsObligation) {
			errMsg = "Nominal pembayaran melebihi total kewajiban yang boleh dibayar pada periode ini (KD-05). Untuk melunasi seluruh sisa pokok, gunakan fitur Pelunasan Dipercepat."
		}
		h.renderPaymentError(w, r, loanID, errMsg)
		return
	}

	http.Redirect(w, r, "/payments/"+strconv.FormatInt(result.Payment.ID, 10)+"/receipt", http.StatusSeeOther)
}

// ShowPaymentReceipt menampilkan bukti kuitansi pembayaran resmi beserta rincian alokasi ke tiap angsuran/komponen.
func (h *Handler) ShowPaymentReceipt(w http.ResponseWriter, r *http.Request) {
	paymentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	receipt, err := h.paymentSvc.GetReceipt(r.Context(), paymentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	h.Render(w, r, "payments/receipt.html", map[string]any{
		"Receipt": receipt,
	})
}

func (h *Handler) renderPaymentError(w http.ResponseWriter, r *http.Request, loanID int64, errMsg string) {
	payDateStr := r.FormValue("payment_date")
	payDate, err := domain.ParseBusinessDate(payDateStr)
	if err != nil {
		payDate = service.BusinessToday()
		payDateStr = payDate.Format("2006-01-02")
	}
	preview, err := h.paymentSvc.Preview(r.Context(), loanID, payDate)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, errMsg)
		return
	}

	h.Render(w, r, "payments/form.html", map[string]any{
		"Error":       errMsg,
		"Preview":     preview,
		"PaymentDate": payDateStr,
		"Amount":      r.FormValue("amount"),
		"RequestKey":  r.FormValue("request_key"),
	})
}
