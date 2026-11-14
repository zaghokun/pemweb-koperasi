package middleware

import (
	"net/http"

	"koperasi/internal/service"
)

// RequireCSRF memverifikasi token CSRF pada method yang mengubah state (POST, PUT, DELETE, PATCH).
func RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			sess := SessionFromContext(r.Context())
			submittedToken := r.FormValue("_csrf")
			if submittedToken == "" {
				submittedToken = r.Header.Get("X-CSRF-Token")
			}

			if !service.VerifyCSRF(sess, submittedToken) {
				http.Error(w, "Forbidden: Token CSRF tidak sah atau tidak ditemukan.", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
