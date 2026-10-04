package middleware

import (
	"context"
	"net/http"

	"topupku/model"
	"topupku/store"
)

const (
	UserContextKey contextKey = "user_account"
)

// UserAuth injects the logged-in *model.User into request context if valid user_session cookie exists.
// Does NOT block unauthenticated requests (guest mode allowed).
func UserAuth(st *store.SQLiteStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("user_session")
			if err == nil && cookie.Value != "" {
				if user, err := st.GetUserBySession(cookie.Value); err == nil && user != nil && user.IsActive {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser blocks request if user is not logged in.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user == nil {
			http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUser retrieves the current logged-in *model.User from context (or nil).
func GetUser(r *http.Request) *model.User {
	if val := r.Context().Value(UserContextKey); val != nil {
		if u, ok := val.(*model.User); ok {
			return u
		}
	}
	return nil
}
