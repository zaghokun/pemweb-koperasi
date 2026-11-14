package middleware

import (
	"net/http"
)

// RequireAuth memastikan hanya pengguna terautentikasi yang dapat mengakses handler selanjutnya.
// Jika belum login, redirect ke /login.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
