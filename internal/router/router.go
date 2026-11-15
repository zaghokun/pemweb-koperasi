package router

import (
	"net/http"

	"koperasi/internal/handler"
	"koperasi/internal/middleware"
	"koperasi/internal/service"
)

// New membuat HTTP handler/mux dengan konfigurasi middleware sesi, autentikasi, dan proteksi CSRF.
func New(h *handler.Handler, authSvc *service.AuthService, cookieSecure bool, staticDir string) http.Handler {
	mux := http.NewServeMux()

	// Aset statis publik (CSS, JS)
	fileServer := http.FileServer(http.Dir(staticDir))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	// Rute publik login
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /login", h.ShowLogin)
	mux.HandleFunc("POST /login", h.Login)

	// Rute terproteksi (harus login)
	mux.Handle("POST /logout", middleware.RequireAuth(http.HandlerFunc(h.Logout)))
	mux.Handle("GET /dashboard", middleware.RequireAuth(http.HandlerFunc(h.ShowDashboard)))

	// Rute Anggota (Tahap 5)
	mux.Handle("GET /members", middleware.RequireAuth(http.HandlerFunc(h.ListMembers)))
	mux.Handle("GET /members/new", middleware.RequireAuth(http.HandlerFunc(h.ShowNewMember)))
	mux.Handle("POST /members", middleware.RequireAuth(http.HandlerFunc(h.CreateMember)))
	mux.Handle("GET /members/{id}", middleware.RequireAuth(http.HandlerFunc(h.ShowMember)))
	mux.Handle("GET /members/{id}/edit", middleware.RequireAuth(http.HandlerFunc(h.ShowEditMember)))
	mux.Handle("POST /members/{id}/edit", middleware.RequireAuth(http.HandlerFunc(h.UpdateMember)))
	mux.Handle("GET /members/{id}/exit", middleware.RequireAuth(http.HandlerFunc(h.ShowExitMember)))
	mux.Handle("POST /members/{id}/exit", middleware.RequireAuth(http.HandlerFunc(h.ExitMember)))

	// Rute Simpanan (Tahap 5)
	mux.Handle("GET /savings", middleware.RequireAuth(http.HandlerFunc(h.ListSavings)))
	mux.Handle("GET /savings/new", middleware.RequireAuth(http.HandlerFunc(h.ShowNewDeposit)))
	mux.Handle("POST /savings", middleware.RequireAuth(http.HandlerFunc(h.CreateDeposit)))
	mux.Handle("GET /savings/withdrawals/new", middleware.RequireAuth(http.HandlerFunc(h.ShowNewWithdrawal)))
	mux.Handle("POST /savings/withdrawals", middleware.RequireAuth(http.HandlerFunc(h.CreateWithdrawal)))
	mux.Handle("GET /savings/{id}/receipt", middleware.RequireAuth(http.HandlerFunc(h.ShowSavingReceipt)))

	// Rute Pinjaman & Jadwal (Tahap 6)
	mux.Handle("GET /loans", middleware.RequireAuth(http.HandlerFunc(h.ListLoans)))
	mux.Handle("GET /loans/new", middleware.RequireAuth(http.HandlerFunc(h.ShowNewLoan)))
	mux.Handle("POST /loans", middleware.RequireAuth(http.HandlerFunc(h.CreateLoan)))
	mux.Handle("GET /loans/{id}", middleware.RequireAuth(http.HandlerFunc(h.ShowLoan)))
	mux.Handle("POST /loans/{id}/approve", middleware.RequireAuth(http.HandlerFunc(h.ApproveLoan)))
	mux.Handle("POST /loans/{id}/reject", middleware.RequireAuth(http.HandlerFunc(h.RejectLoan)))
	mux.Handle("POST /loans/{id}/disburse", middleware.RequireAuth(http.HandlerFunc(h.DisburseLoan)))
	mux.Handle("GET /loans/{id}/installments", middleware.RequireAuth(http.HandlerFunc(h.ShowSchedule)))

	// Payment routes (Tahap 6)
	mux.Handle("GET /loans/{id}/payments/new", middleware.RequireAuth(http.HandlerFunc(h.ShowNewPayment)))
	mux.Handle("POST /loans/{id}/payments", middleware.RequireAuth(http.HandlerFunc(h.CreatePayment)))
	mux.Handle("GET /payments/{id}/receipt", middleware.RequireAuth(http.HandlerFunc(h.ShowPaymentReceipt)))

	// Terapkan stack middleware global: CSRF protection -> Session loader
	// Urutan eksekusi: SessionMiddleware berjalan lebih dahulu agar context berisi sesi & user,
	// kemudian RequireCSRF memeriksa token jika method POST/PUT/DELETE.
	csrfHandler := middleware.RequireCSRF(mux)
	sessionHandler := middleware.SessionMiddleware(authSvc, cookieSecure)(csrfHandler)

	return sessionHandler
}
