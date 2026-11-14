package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"koperasi/internal/domain"
	"koperasi/internal/middleware"
	"koperasi/internal/service"
)

// ListMembers menampilkan daftar anggota dengan pencarian dan pagination.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("q")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	members, total, err := h.memberSvc.List(r.Context(), search, page, 20)
	if err != nil {
		http.Error(w, "Gagal memuat daftar anggota", http.StatusInternalServerError)
		return
	}

	h.Render(w, r, "members/index.html", map[string]any{
		"Members":      members,
		"TotalMembers": total,
		"Search":       search,
		"Page":         page,
	})
}

// ShowNewMember menampilkan formulir pendaftaran anggota baru dan setoran pokok awal.
func (h *Handler) ShowNewMember(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("2006-01-02")
	h.Render(w, r, "members/form.html", map[string]any{
		"IsEdit":   false,
		"JoinDate": today,
	})
}

// CreateMember memproses pendaftaran anggota dan pencatatan simpanan pokok Rp500.000 secara atomik.
func (h *Handler) CreateMember(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	memberNum := r.FormValue("member_number")
	name := r.FormValue("name")
	address := r.FormValue("address")
	phone := r.FormValue("phone")
	joinDateStr := r.FormValue("join_date")

	joinDate, err := domain.ParseBusinessDate(joinDateStr)
	if err != nil {
		h.Render(w, r, "members/form.html", map[string]any{
			"IsEdit":       false,
			"Error":        "Format tanggal masuk tidak valid (gunakan YYYY-MM-DD)",
			"MemberNumber": memberNum,
			"Name":         name,
			"Address":      address,
			"Phone":        phone,
			"JoinDate":     joinDateStr,
		})
		return
	}

	member, _, err := h.memberSvc.Register(r.Context(), service.RegisterMemberInput{
		MemberNumber: memberNum,
		Name:         name,
		Address:      address,
		Phone:        phone,
		JoinDate:     joinDate,
		UserID:       user.ID,
	})
	if err != nil {
		errMsg := "Gagal mendaftarkan anggota: " + err.Error()
		if errors.Is(err, service.ErrMemberNumberExists) {
			errMsg = "Nomor anggota sudah terdaftar, gunakan nomor unik."
		}
		h.Render(w, r, "members/form.html", map[string]any{
			"IsEdit":       false,
			"Error":        errMsg,
			"MemberNumber": memberNum,
			"Name":         name,
			"Address":      address,
			"Phone":        phone,
			"JoinDate":     joinDateStr,
		})
		return
	}

	http.Redirect(w, r, "/members/"+strconv.FormatInt(member.ID, 10), http.StatusSeeOther)
}

// ShowMember menampilkan detail profil anggota, saldo per jenis, batas pinjaman, dan riwayat simpanan.
func (h *Handler) ShowMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	detail, err := h.memberSvc.GetDetail(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrMemberNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Gagal memuat detail anggota", http.StatusInternalServerError)
		return
	}

	savings, _, _ := h.savingSvc.List(r.Context(), &id, nil, 1, 20)

	h.Render(w, r, "members/detail.html", map[string]any{
		"Detail":  detail,
		"Savings": savings,
	})
}

// ShowEditMember menampilkan formulir ubah identitas anggota (nama, alamat, telepon).
func (h *Handler) ShowEditMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	detail, err := h.memberSvc.GetDetail(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	h.Render(w, r, "members/form.html", map[string]any{
		"IsEdit":       true,
		"MemberID":     detail.Member.ID,
		"MemberNumber": detail.Member.MemberNumber,
		"Name":         detail.Member.Name,
		"Address":      detail.Member.Address,
		"Phone":        detail.Member.Phone,
		"JoinDate":     detail.Member.JoinDate.Format("2006-01-02"),
	})
}

// UpdateMember memproses pengubahan identitas anggota.
func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	name := r.FormValue("name")
	address := r.FormValue("address")
	phone := r.FormValue("phone")

	if err := h.memberSvc.UpdateIdentity(r.Context(), id, name, address, phone); err != nil {
		http.Error(w, "Gagal memperbarui identitas anggota: "+err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/members/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// ShowExitMember menampilkan pratinjau keluar anggota dan total saldo yang akan dikembalikan.
func (h *Handler) ShowExitMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	detail, err := h.memberSvc.GetDetail(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	today := time.Now().Format("2006-01-02")
	h.Render(w, r, "members/exit.html", map[string]any{
		"Detail":   detail,
		"ExitDate": today,
	})
}

// ExitMember memproses pengeluaran anggota secara permanen dan mencatat pengembalian simpanan.
func (h *Handler) ExitMember(w http.ResponseWriter, r *http.Request) {
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

	exitDateStr := r.FormValue("exit_date")
	exitDate, err := domain.ParseBusinessDate(exitDateStr)
	if err != nil {
		detail, _ := h.memberSvc.GetDetail(r.Context(), id)
		h.Render(w, r, "members/exit.html", map[string]any{
			"Detail":   detail,
			"Error":    "Format tanggal keluar tidak valid (gunakan YYYY-MM-DD)",
			"ExitDate": exitDateStr,
		})
		return
	}

	_, err = h.memberSvc.Exit(r.Context(), id, exitDate, user.ID)
	if err != nil {
		detail, _ := h.memberSvc.GetDetail(r.Context(), id)
		errMsg := "Gagal memproses keluar anggota: " + err.Error()
		if errors.Is(err, service.ErrMemberHasOpenLoan) {
			errMsg = "Anggota tidak dapat keluar karena masih memiliki pinjaman aktif atau pengajuan terbuka."
		}
		h.Render(w, r, "members/exit.html", map[string]any{
			"Detail":   detail,
			"Error":    errMsg,
			"ExitDate": exitDateStr,
		})
		return
	}

	http.Redirect(w, r, "/members/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
