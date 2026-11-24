package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/repository"
	"koperasi/internal/service"
)

// ShowDashboard menampilkan halaman ringkasan operasional dengan metrik agregat riil.
func (h *Handler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	today := service.BusinessToday()
	var stats *repository.DashboardSummary
	if h.reportSvc != nil {
		var err error
		stats, err = h.reportSvc.DashboardStats(r.Context(), today)
		if err != nil {
			h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Gagal memuat statistik dashboard: %v", err))
			return
		}
	}
	if stats == nil {
		stats = &repository.DashboardSummary{}
	}

	data := h.newTemplateData(r)
	data["Stats"] = stats
	data["Today"] = today.Format("2006-01-02")

	h.render(w, r, "dashboard/index.html", data)
}

// ShowLoanCard menampilkan kartu pinjaman lengkap beserta histori pembayaran dan jadwalnya.
func (h *Handler) ShowLoanCard(w http.ResponseWriter, r *http.Request) {
	loanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.renderNotFound(w, r)
		return
	}

	asOf := service.BusinessToday()
	if date := r.URL.Query().Get("as_of"); date != "" {
		asOf, err = domain.ParseBusinessDate(date)
		if err != nil {
			h.renderError(w, r, http.StatusBadRequest, "Tanggal posisi laporan tidak valid")
			return
		}
	}

	card, err := h.reportSvc.LoanCard(r.Context(), loanID, asOf)
	if err != nil {
		h.renderNotFound(w, r)
		return
	}

	data := h.newTemplateData(r)
	data["Card"] = card
	data["AsOfDate"] = asOf.Format("2006-01-02")
	data["PositionDate"] = asOf

	h.render(w, r, "reports/loan_card.html", data)
}

// ShowArrearsReport menampilkan daftar seluruh pinjaman aktif yang menunggak per tanggal tertentu.
func (h *Handler) ShowArrearsReport(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("as_of")
	var asOf time.Time
	var err error

	if dateStr != "" {
		asOf, err = domain.ParseBusinessDate(dateStr)
		if err != nil {
			asOf = service.BusinessToday()
		}
	} else {
		asOf = service.BusinessToday()
	}

	items, err := h.reportSvc.ArrearsReport(r.Context(), asOf)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Gagal memuat laporan tunggakan: %v", err))
		return
	}

	var totalPrincipal, totalInterest, totalPenalty, grandTotal int64
	for _, it := range items {
		totalPrincipal += it.PrincipalOverdue
		totalInterest += it.InterestOverdue
		totalPenalty += it.PenaltyDue
		grandTotal += it.TotalArrears
	}

	data := h.newTemplateData(r)
	data["Items"] = items
	data["AsOfDate"] = asOf.Format("2006-01-02")
	data["TotalPrincipal"] = totalPrincipal
	data["TotalInterest"] = totalInterest
	data["TotalPenalty"] = totalPenalty
	data["GrandTotal"] = grandTotal

	h.render(w, r, "reports/arrears.html", data)
}

// ShowMonthlyIncomeReport menampilkan rekapitulasi penerimaan kas jasa dan denda bulanan.
func (h *Handler) ShowMonthlyIncomeReport(w http.ResponseWriter, r *http.Request) {
	now := service.BusinessToday()
	year := now.Year()
	month := int(now.Month())

	if yStr := r.URL.Query().Get("year"); yStr != "" {
		if y, err := strconv.Atoi(yStr); err == nil && y >= 2020 && y <= 2100 {
			year = y
		}
	}
	if mStr := r.URL.Query().Get("month"); mStr != "" {
		if m, err := strconv.Atoi(mStr); err == nil && m >= 1 && m <= 12 {
			month = m
		}
	}

	summary, err := h.reportSvc.MonthlyIncome(r.Context(), year, month)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, fmt.Sprintf("Gagal memuat laporan penerimaan bulanan: %v", err))
		return
	}

	data := h.newTemplateData(r)
	data["Summary"] = summary
	data["SelectedYear"] = year
	data["SelectedMonth"] = month

	h.render(w, r, "reports/monthly.html", data)
}
