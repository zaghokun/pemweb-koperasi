package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"koperasi/internal/domain"
	"koperasi/internal/middleware"
	"koperasi/internal/service"
)

// ListSavings menampilkan riwayat mutasi simpanan dengan filter anggota dan jenis simpanan.
func (h *Handler) ListSavings(w http.ResponseWriter, r *http.Request) {
	page := listPage(r)

	var memberID *int64
	if midStr := r.URL.Query().Get("member_id"); midStr != "" {
		if id, err := strconv.ParseInt(midStr, 10, 64); err == nil {
			memberID = &id
		}
	}

	var savingType *domain.SavingType
	if stStr := r.URL.Query().Get("type"); stStr != "" {
		st := domain.SavingType(strings.ToUpper(stStr))
		savingType = &st
	}

	savings, total, err := h.savingSvc.List(r.Context(), memberID, savingType, page, listPageSize)
	if err != nil {
		http.Error(w, "Gagal memuat riwayat simpanan", http.StatusInternalServerError)
		return
	}

	h.Render(w, r, "savings/index.html", map[string]any{
		"Savings":    savings,
		"TotalCount": total,
		"Pagination": pagination(r, page, total),
		"MemberID":   r.URL.Query().Get("member_id"),
		"SavingType": r.URL.Query().Get("type"),
	})
}

// ShowNewDeposit menampilkan form setoran simpanan (Wajib atau Sukarela).
func (h *Handler) ShowNewDeposit(w http.ResponseWriter, r *http.Request) {
	reqKey := generateRequestKey()
	today := service.BusinessToday().Format("2006-01-02")
	currentMonth := service.BusinessToday().Format("2006-01")

	memberID := r.URL.Query().Get("member_id")

	h.Render(w, r, "savings/deposit.html", map[string]any{
		"RequestKey":      reqKey,
		"TransactionDate": today,
		"PeriodMonth":     currentMonth,
		"MemberID":        memberID,
	})
}

// CreateDeposit memproses setoran simpanan wajib atau sukarela.
func (h *Handler) CreateDeposit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	memberID, err := strconv.ParseInt(r.FormValue("member_id"), 10, 64)
	if err != nil {
		h.renderDepositError(w, r, "ID anggota tidak valid", r.FormValue("saving_type"))
		return
	}

	savingType := r.FormValue("saving_type")
	txDateStr := r.FormValue("transaction_date")
	reqKey := r.FormValue("request_key")
	desc := r.FormValue("description")

	txDate, err := domain.ParseBusinessDate(txDateStr)
	if err != nil {
		h.renderDepositError(w, r, "Format tanggal transaksi tidak valid (gunakan YYYY-MM-DD)", savingType)
		return
	}

	var saving *domain.Saving

	switch savingType {
	case "WAJIB":
		periodStr := r.FormValue("period_month") // format YYYY-MM
		if len(periodStr) != 7 {
			h.renderDepositError(w, r, "Format periode wajib tidak valid (gunakan YYYY-MM)", savingType)
			return
		}
		periodDate, err := domain.ParseBusinessDate(periodStr + "-01")
		if err != nil {
			h.renderDepositError(w, r, "Format periode bulan tidak valid", savingType)
			return
		}

		s, err := h.savingSvc.DepositMandatory(r.Context(), service.DepositMandatoryInput{
			MemberID:        memberID,
			UserID:          user.ID,
			PeriodMonth:     periodDate,
			TransactionDate: txDate,
			RequestKey:      reqKey,
			Description:     desc,
		})
		if err != nil {
			errMsg := "Gagal mencatat simpanan wajib: " + err.Error()
			if errors.Is(err, service.ErrDuplicateMandatoryPlan) {
				errMsg = "Simpanan wajib untuk periode " + periodStr + " sudah pernah disetor."
			} else if errors.Is(err, service.ErrInvalidPeriod) {
				errMsg = "Periode simpanan wajib tidak boleh sebelum bulan anggota bergabung."
			}
			h.renderDepositError(w, r, errMsg, savingType)
			return
		}
		saving = s

	case "SUKARELA":
		amountStr := r.FormValue("amount")
		amount, err := domain.ParseRupiah(amountStr)
		if err != nil {
			h.renderDepositError(w, r, "Nominal simpanan sukarela tidak valid (gunakan digit rupiah bulat positif)", savingType)
			return
		}

		s, err := h.savingSvc.DepositVoluntary(r.Context(), service.DepositVoluntaryInput{
			MemberID:        memberID,
			UserID:          user.ID,
			Amount:          amount,
			TransactionDate: txDate,
			RequestKey:      reqKey,
			Description:     desc,
		})
		if err != nil {
			h.renderDepositError(w, r, "Gagal mencatat simpanan sukarela: "+err.Error(), savingType)
			return
		}
		saving = s

	default:
		h.renderDepositError(w, r, "Jenis simpanan tidak valid. Setoran pokok hanya dilakukan melalui pendaftaran anggota.", savingType)
		return
	}

	http.Redirect(w, r, "/savings/"+strconv.FormatInt(saving.ID, 10)+"/receipt", http.StatusSeeOther)
}

// ShowNewWithdrawal menampilkan form penarikan simpanan sukarela.
func (h *Handler) ShowNewWithdrawal(w http.ResponseWriter, r *http.Request) {
	reqKey := generateRequestKey()
	today := service.BusinessToday().Format("2006-01-02")
	memberID := r.URL.Query().Get("member_id")

	var sukarelaBalance int64
	if mid, err := strconv.ParseInt(memberID, 10, 64); err == nil {
		detail, err := h.memberSvc.GetDetail(r.Context(), mid)
		if err == nil {
			sukarelaBalance = detail.SukarelaBalance
		}
	}

	h.Render(w, r, "savings/withdrawal.html", map[string]any{
		"RequestKey":      reqKey,
		"TransactionDate": today,
		"MemberID":        memberID,
		"SukarelaBalance": sukarelaBalance,
	})
}

// CreateWithdrawal memproses penarikan simpanan sukarela dengan validasi saldo di bawah row lock.
func (h *Handler) CreateWithdrawal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	memberID, err := strconv.ParseInt(r.FormValue("member_id"), 10, 64)
	if err != nil {
		h.renderWithdrawalError(w, r, "ID anggota tidak valid", 0)
		return
	}

	txDateStr := r.FormValue("transaction_date")
	reqKey := r.FormValue("request_key")
	desc := r.FormValue("description")
	amountStr := r.FormValue("amount")

	txDate, err := domain.ParseBusinessDate(txDateStr)
	if err != nil {
		h.renderWithdrawalError(w, r, "Format tanggal tidak valid", memberID)
		return
	}

	amount, err := domain.ParseRupiah(amountStr)
	if err != nil {
		h.renderWithdrawalError(w, r, "Nominal penarikan tidak valid (gunakan digit rupiah bulat positif)", memberID)
		return
	}

	saving, err := h.savingSvc.WithdrawVoluntary(r.Context(), service.WithdrawVoluntaryInput{
		MemberID:        memberID,
		UserID:          user.ID,
		Amount:          amount,
		TransactionDate: txDate,
		RequestKey:      reqKey,
		Description:     desc,
	})
	if err != nil {
		errMsg := "Gagal memproses penarikan: " + err.Error()
		if errors.Is(err, service.ErrInsufficientBalance) {
			errMsg = "Saldo simpanan sukarela tidak mencukupi untuk melakukan penarikan ini."
		}
		h.renderWithdrawalError(w, r, errMsg, memberID)
		return
	}

	http.Redirect(w, r, "/savings/"+strconv.FormatInt(saving.ID, 10)+"/receipt", http.StatusSeeOther)
}

// ShowSavingReceipt menampilkan bukti kuitansi transaksi simpanan atau penarikan ramah cetak.
func (h *Handler) ShowSavingReceipt(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	saving, member, err := h.savingSvc.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	h.Render(w, r, "savings/receipt.html", map[string]any{
		"Saving": saving,
		"Member": member,
	})
}

func (h *Handler) renderDepositError(w http.ResponseWriter, r *http.Request, errMsg, savingType string) {
	h.Render(w, r, "savings/deposit.html", map[string]any{
		"Error":           errMsg,
		"SavingType":      savingType,
		"MemberID":        r.FormValue("member_id"),
		"TransactionDate": r.FormValue("transaction_date"),
		"PeriodMonth":     r.FormValue("period_month"),
		"Amount":          r.FormValue("amount"),
		"RequestKey":      r.FormValue("request_key"),
		"Description":     r.FormValue("description"),
	})
}

func (h *Handler) renderWithdrawalError(w http.ResponseWriter, r *http.Request, errMsg string, memberID int64) {
	var sukarelaBalance int64
	if memberID > 0 {
		detail, _ := h.memberSvc.GetDetail(r.Context(), memberID)
		if detail != nil {
			sukarelaBalance = detail.SukarelaBalance
		}
	}
	h.Render(w, r, "savings/withdrawal.html", map[string]any{
		"Error":           errMsg,
		"MemberID":        strconv.FormatInt(memberID, 10),
		"TransactionDate": r.FormValue("transaction_date"),
		"Amount":          r.FormValue("amount"),
		"RequestKey":      r.FormValue("request_key"),
		"Description":     r.FormValue("description"),
		"SukarelaBalance": sukarelaBalance,
	})
}

func generateRequestKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
