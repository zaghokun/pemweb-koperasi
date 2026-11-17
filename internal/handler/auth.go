package handler

import (
	"net/http"

	"koperasi/internal/middleware"
)

// ShowLogin menampilkan formulir login dan memastikan sesi pra-login serta CSRF tersedia.
func (h *Handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if user := middleware.UserFromContext(r.Context()); user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	csrfToken := ""
	sess := middleware.SessionFromContext(r.Context())
	if sess == nil {
		token, csrf, err := h.authSvc.CreatePreLoginSession(r.Context())
		if err != nil {
			http.Error(w, "Gagal membuat sesi login", http.StatusInternalServerError)
			return
		}
		middleware.SetSessionCookie(w, token, h.cookieSecure)
		csrfToken = csrf
	} else {
		csrfToken = sess.CSRFToken
	}

	h.Render(w, r, "auth/login.html", map[string]any{
		"CSRFToken": csrfToken,
	})
}

// Login memproses autentikasi form login, merotasi sesi, dan mengarahkan ke dashboard.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	var oldToken string
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		oldToken = cookie.Value
	}

	newToken, _, _, err := h.authSvc.Login(r.Context(), oldToken, username, password)
	if err != nil {
		// Buat sesi pra-login baru agar form login berikutnya memiliki CSRF valid
		newPreToken, preCSRF, preErr := h.authSvc.CreatePreLoginSession(r.Context())
		if preErr == nil {
			middleware.SetSessionCookie(w, newPreToken, h.cookieSecure)
		}
		h.Render(w, r, "auth/login.html", map[string]any{
			"Error":     "Username atau password salah.",
			"Username":  username,
			"CSRFToken": preCSRF,
		})
		return
	}

	middleware.SetSessionCookie(w, newToken, h.cookieSecure)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Logout mencabut sesi pengurus dan menghapus cookie di browser.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil && cookie.Value != "" {
		_ = h.authSvc.Logout(r.Context(), cookie.Value)
	}
	middleware.ClearSessionCookie(w, h.cookieSecure)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
