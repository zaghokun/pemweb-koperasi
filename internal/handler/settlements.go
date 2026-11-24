package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/middleware"
	"koperasi/internal/repository"
	"koperasi/internal/service"
)

// ShowSettlementPreview menampilkan formulir dan kalkulasi pratinjau pelunasan dipercepat (KD-07, KD-08, KD-09).
func (h *Handler) ShowSettlementPreview(w http.ResponseWriter, r *http.Request) {
	loanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.renderNotFound(w, r)
		return
	}

	dateStr := r.URL.Query().Get("date")
	var targetDate time.Time
	if dateStr != "" {
		targetDate, err = domain.ParseBusinessDate(dateStr)
		if err != nil {
			targetDate = service.BusinessToday()
		}
	} else {
		targetDate = service.BusinessToday()
	}

	data := h.newTemplateData(r)
	data["LoanID"] = loanID
	data["TargetDate"] = targetDate.Format("2006-01-02")
	preview, err := h.settlementSvc.Preview(r.Context(), loanID, targetDate)
	if errors.Is(err, repository.ErrNotFound) {
		h.renderNotFound(w, r)
		return
	}
	if err != nil {
		data["FlashError"] = fmt.Sprintf("Gagal menghitung pratinjau pelunasan: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
	}
	data["Preview"] = preview
	data["RequestKey"] = generateRequestKey()

	h.render(w, r, "settlements/preview.html", data)
}

// ProcessSettlement mengeksekusi pelunasan dipercepat secara atomik di server.
func (h *Handler) ProcessSettlement(w http.ResponseWriter, r *http.Request) {
	loanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.renderNotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Form tidak valid")
		return
	}

	dateStr := r.FormValue("settlement_date")
	settlementDate, err := domain.ParseBusinessDate(dateStr)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Tanggal pelunasan tidak valid")
		return
	}

	confirmedAmount, err := strconv.ParseInt(r.FormValue("confirmed_amount"), 10, 64)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Nominal konfirmasi tidak valid")
		return
	}

	reqKey := r.FormValue("request_key")
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	res, err := h.settlementSvc.Settle(r.Context(), service.SettleInput{
		LoanID:          loanID,
		UserID:          user.ID,
		SettlementDate:  settlementDate,
		ConfirmedAmount: confirmedAmount,
		RequestKey:      reqKey,
	})
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, fmt.Sprintf("Gagal memproses pelunasan dipercepat: %v", err))
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/settlements/%d/receipt", res.Payment.ID), http.StatusSeeOther)
}

// ShowSettlementReceipt menampilkan kuitansi pelunasan dipercepat ramah cetak.
func (h *Handler) ShowSettlementReceipt(w http.ResponseWriter, r *http.Request) {
	paymentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.renderNotFound(w, r)
		return
	}

	receipt, err := h.settlementSvc.GetReceipt(r.Context(), paymentID)
	if err != nil {
		h.renderNotFound(w, r)
		return
	}

	data := h.newTemplateData(r)
	data["Receipt"] = receipt

	h.render(w, r, "settlements/receipt.html", data)
}
