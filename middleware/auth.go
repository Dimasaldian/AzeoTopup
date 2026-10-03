package middleware

import (
	"context"
	"net/http"

	"topupku/model"
	"topupku/store"
)

type contextKey string

const (
	AdminUserContextKey contextKey = "admin_user"
	SessionContextKey   contextKey = "admin_session"
)

func RequireAdmin(st *store.SQLiteStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("admin_session")
			if err != nil || cookie.Value == "" {
				http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
				return
			}

			sess, user, err := st.GetSession(cookie.Value)
			if err != nil || user == nil || !user.IsActive {
				// Expired or invalid session
				http.SetCookie(w, &http.Cookie{
					Name:     "admin_session",
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
				})
				http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), AdminUserContextKey, user)
			ctx = context.WithValue(ctx, SessionContextKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAdminUser(r *http.Request) *model.AdminUser {
	if val := r.Context().Value(AdminUserContextKey); val != nil {
		if user, ok := val.(*model.AdminUser); ok {
			return user
		}
	}
	return nil
}
