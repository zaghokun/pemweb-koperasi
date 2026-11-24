package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"koperasi/internal/domain"
	"koperasi/internal/middleware"
	"koperasi/internal/service"
)

// ListLoans menampilkan daftar pinjaman dengan filter status dan anggota.
func (h *Handler) ListLoans(w http.ResponseWriter, r *http.Request) {
	page := listPage(r)

	var memberID *int64
	if midStr := r.URL.Query().Get("member_id"); midStr != "" {
		if id, err := strconv.ParseInt(midStr, 10, 64); err == nil {
			memberID = &id
		}
	}

	var status *domain.LoanStatus
	if stStr := r.URL.Query().Get("status"); stStr != "" {
		st := domain.LoanStatus(strings.ToUpper(stStr))
		status = &st
	}

	loans, total, err := h.loanSvc.List(r.Context(), memberID, status, page, listPageSize)
	if err != nil {
		http.Error(w, "Gagal memuat daftar pinjaman", http.StatusInternalServerError)
		return
	}

	h.Render(w, r, "loans/index.html", map[string]any{
		"Loans":      loans,
		"TotalCount": total,
		"Pagination": pagination(r, page, total),
		"MemberID":   r.URL.Query().Get("member_id"),
		"Status":     r.URL.Query().Get("status"),
	})
}

// ShowNewLoan menampilkan form pengajuan pinjaman baru beserta batas 3x simpanan pokok dan wajib.
func (h *Handler) ShowNewLoan(w http.ResponseWriter, r *http.Request) {
	memberIDStr := r.URL.Query().Get("member_id")
	today := service.BusinessToday().Format("2006-01-02")

	var member *domain.Member
	var maxLimit int64

	if mid, err := strconv.ParseInt(memberIDStr, 10, 64); err == nil {
		detail, err := h.memberSvc.GetDetail(r.Context(), mid)
		if err == nil {
			member = &detail.Member
			maxLimit = detail.MaxLoanLimit
		}
	}

	h.Render(w, r, "loans/form.html", map[string]any{
		"MemberID":        memberIDStr,
		"Member":          member,
		"MaxLimit":        maxLimit,
		"ApplicationDate": today,
		"MinTenor":        domain.MinTenorMonths,
		"MaxTenor":        domain.MaxTenorMonths,
	})
}

// CreateLoan memproses pengajuan pinjaman baru.
func (h *Handler) CreateLoan(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	memberID, err := strconv.ParseInt(r.FormValue("member_id"), 10, 64)
	if err != nil {
		h.renderLoanFormError(w, r, "ID anggota tidak valid", 0)
		return
	}

	principalStr := r.FormValue("principal")
	principal, err := domain.ParseRupiah(principalStr)
	if err != nil {
		h.renderLoanFormError(w, r, "Nominal pinjaman harus berupa bilangan bulat rupiah positif", memberID)
		return
	}

	tenor, err := strconv.Atoi(r.FormValue("tenor_months"))
	if err != nil || tenor < domain.MinTenorMonths || tenor > domain.MaxTenorMonths {
		h.renderLoanFormError(w, r, "Tenor pinjaman harus berupa bilangan bulat antara 6 sampai 24 bulan", memberID)
		return
	}

	appDateStr := r.FormValue("application_date")
	appDate, err := domain.ParseBusinessDate(appDateStr)
	if err != nil {
		h.renderLoanFormError(w, r, "Format tanggal pengajuan tidak valid (gunakan YYYY-MM-DD)", memberID)
		return
	}

	loan, err := h.loanSvc.Apply(r.Context(), service.ApplyLoanInput{
		MemberID:        memberID,
		Principal:       principal,
		TenorMonths:     tenor,
		ApplicationDate: appDate,
		UserID:          user.ID,
	})
	if err != nil {
		errMsg := "Gagal mengajukan pinjaman: " + err.Error()
		if errors.Is(err, service.ErrExceedsMaxLoanLimit) {
			errMsg = "Nominal pinjaman melebihi batas maksimum (3x saldo simpanan pokok dan wajib)."
		} else if errors.Is(err, service.ErrMemberHasOpenLoan) {
			errMsg = "Anggota masih memiliki proses pinjaman terbuka (PENDING, APPROVED, atau ACTIVE)."
		}
		h.renderLoanFormError(w, r, errMsg, memberID)
		return
	}

	http.Redirect(w, r, "/loans/"+strconv.FormatInt(loan.ID, 10), http.StatusSeeOther)
}

// ShowLoan menampilkan detail status pinjaman dan tombol aksi sesuai alur state machine.
func (h *Handler) ShowLoan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	detail, err := h.loanSvc.GetDetail(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrLoanNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Gagal memuat detail pinjaman", http.StatusInternalServerError)
		return
	}

	today := service.BusinessToday().Format("2006-01-02")
	h.Render(w, r, "loans/detail.html", map[string]any{
		"Detail":   detail,
		"Today":    today,
		"MinTenor": domain.MinTenorMonths,
		"MaxTenor": domain.MaxTenorMonths,
	})
}

// ApproveLoan menyetujui pengajuan pinjaman yang berstatus PENDING.
func (h *Handler) ApproveLoan(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.loanSvc.Approve(r.Context(), id, user.ID); err != nil {
		http.Error(w, "Gagal menyetujui pinjaman: "+err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/loans/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// RejectLoan menolak pinjaman PENDING atau membatalkan APPROVED dengan alasan.
func (h *Handler) RejectLoan(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	reason := r.FormValue("rejection_reason")
	if err := h.loanSvc.Reject(r.Context(), id, reason, user.ID); err != nil {
		http.Error(w, "Gagal menolak pinjaman: "+err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/loans/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// DisburseLoan mencairkan pinjaman APPROVED, membuat jadwal angsuran, dan mengubah status ke ACTIVE.
func (h *Handler) DisburseLoan(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	disburseDateStr := r.FormValue("disbursement_date")
	disburseDate, err := domain.ParseBusinessDate(disburseDateStr)
	if err != nil {
		http.Error(w, "Format tanggal pencairan tidak valid", http.StatusBadRequest)
		return
	}

	if _, err := h.loanSvc.Disburse(r.Context(), id, disburseDate, user.ID); err != nil {
		http.Error(w, "Gagal mencairkan pinjaman: "+err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/loans/"+strconv.FormatInt(id, 10)+"/installments", http.StatusSeeOther)
}

// ShowSchedule menampilkan tabel jadwal angsuran pinjaman (jatuh tempo tanggal 10 tiap bulan).
func (h *Handler) ShowSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	detail, err := h.loanSvc.GetDetail(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	h.Render(w, r, "loans/schedule.html", map[string]any{
		"Detail": detail,
	})
}

func (h *Handler) renderLoanFormError(w http.ResponseWriter, r *http.Request, errMsg string, memberID int64) {
	var member *domain.Member
	var maxLimit int64
	if memberID > 0 {
		detail, _ := h.memberSvc.GetDetail(r.Context(), memberID)
		if detail != nil {
			member = &detail.Member
			maxLimit = detail.MaxLoanLimit
		}
	}
	h.Render(w, r, "loans/form.html", map[string]any{
		"Error":           errMsg,
		"MemberID":        strconv.FormatInt(memberID, 10),
		"Member":          member,
		"MaxLimit":        maxLimit,
		"Principal":       r.FormValue("principal"),
		"TenorMonths":     r.FormValue("tenor_months"),
		"ApplicationDate": r.FormValue("application_date"),
		"MinTenor":        domain.MinTenorMonths,
		"MaxTenor":        domain.MaxTenorMonths,
	})
}
