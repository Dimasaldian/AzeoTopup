package admin

import (
	"net/http"

	"topupku/handler"
	"topupku/service"
	"topupku/store"
)

type AuthHandler struct {
	store      *store.SQLiteStore
	adminSvc   *service.AdminService
	sessionAge int
}

func NewAuthHandler(st *store.SQLiteStore, as *service.AdminService, sessionAge int) *AuthHandler {
	return &AuthHandler{
		store:      st,
		adminSvc:   as,
		sessionAge: sessionAge,
	}
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to /admin
	if cookie, err := r.Cookie("admin_session"); err == nil && cookie.Value != "" {
		if _, user, err := h.store.GetSession(cookie.Value); err == nil && user != nil {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
	}

	data := map[string]interface{}{
		"Title": "Login Admin — Azeotopup",
		"Error": r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/login.html",
	}, data)
}

func (h *AuthHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/login?error=Invalid+form", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, token, err := h.adminSvc.Authenticate(username, password)
	if err != nil {
		http.Redirect(w, r, "/admin/login?error=Username+atau+password+salah", http.StatusSeeOther)
		return
	}

	// Create session in database
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = forwarded
	}
	_ = h.store.CreateSession(token, user.ID, clientIP, h.sessionAge)

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    token,
		Path:     "/",
		MaxAge:   h.sessionAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	_ = h.store.CreateAuditLog(user.ID, "admin.login", "user", username, "", "Logged in")

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("admin_session"); err == nil && cookie.Value != "" {
		_ = h.store.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}
