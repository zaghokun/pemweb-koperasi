package middleware

import (
	"context"
	"net/http"

	"koperasi/internal/domain"
	"koperasi/internal/service"
)

type contextKey string

const (
	SessionCookieName            = "koperasi_session"
	sessionCtxKey     contextKey = "koperasi_session_ctx"
	userCtxKey        contextKey = "koperasi_user_ctx"
)

// SessionMiddleware memuat sesi dan pengguna dari cookie ke context request.
func SessionMiddleware(authSvc *service.AuthService, cookieSecure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				// Tidak ada cookie sesi
				next.ServeHTTP(w, r)
				return
			}

			sess, user, err := authSvc.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				// Sesi tidak valid atau kedaluwarsa: hapus cookie usang
				ClearSessionCookie(w, cookieSecure)
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			if sess != nil {
				ctx = context.WithValue(ctx, sessionCtxKey, sess)
			}
			if user != nil {
				ctx = context.WithValue(ctx, userCtxKey, user)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionFromContext mengambil *domain.Session dari context request.
func SessionFromContext(ctx context.Context) *domain.Session {
	val := ctx.Value(sessionCtxKey)
	if sess, ok := val.(*domain.Session); ok {
		return sess
	}
	return nil
}

// UserFromContext mengambil *domain.User dari context request.
func UserFromContext(ctx context.Context) *domain.User {
	val := ctx.Value(userCtxKey)
	if u, ok := val.(*domain.User); ok {
		return u
	}
	return nil
}

// SetSessionCookie menyetel cookie sesi dengan atribut aman (HttpOnly, SameSite=Lax, Secure).
func SetSessionCookie(w http.ResponseWriter, token string, cookieSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(service.SessionDuration.Seconds()),
	})
}

// ClearSessionCookie menghapus cookie sesi di browser.
func ClearSessionCookie(w http.ResponseWriter, cookieSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
