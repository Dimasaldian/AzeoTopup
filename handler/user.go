package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"topupku/middleware"
	"topupku/service"
	"topupku/store"
)

type UserHandler struct {
	store      *store.SQLiteStore
	userSvc    *service.UserService
	sessionAge int // seconds (e.g. 30 days = 2592000)
}

func NewUserHandler(st *store.SQLiteStore, us *service.UserService, sessionAge int) *UserHandler {
	if sessionAge <= 0 {
		sessionAge = 86400 * 30 // 30 days
	}
	return &UserHandler{
		store:      st,
		userSvc:    us,
		sessionAge: sessionAge,
	}
}

func (h *UserHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if u := middleware.GetUser(r); u != nil {
		redirect := r.URL.Query().Get("redirect")
		if redirect == "" {
			redirect = "/account"
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}

	data := map[string]interface{}{
		"Title":       "Masuk ke Akun — Azeotopup",
		"RedirectURL": r.URL.Query().Get("redirect"),
		"Error":       r.URL.Query().Get("error"),
		"Message":     r.URL.Query().Get("msg"),
		"User":        nil,
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/user_login.html",
	}, data)
}

func (h *UserHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=Form+tidak+valid", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	redirect := r.FormValue("redirect")
	if redirect == "" || !strings.HasPrefix(redirect, "/") {
		redirect = "/account"
	}

	_, token, err := h.userSvc.Login(email, password, h.sessionAge)
	if err != nil {
		redirURL := fmt.Sprintf("/login?error=%s", strings.ReplaceAll(err.Error(), " ", "+"))
		if redirect != "/account" {
			redirURL += "&redirect=" + redirect
		}
		http.Redirect(w, r, redirURL, http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    token,
		Path:     "/",
		MaxAge:   h.sessionAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *UserHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	if u := middleware.GetUser(r); u != nil {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}

	data := map[string]interface{}{
		"Title":       "Daftar Akun Baru — Azeotopup",
		"RedirectURL": r.URL.Query().Get("redirect"),
		"Error":       r.URL.Query().Get("error"),
		"User":        nil,
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/user_register.html",
	}, data)
}

func (h *UserHandler) RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/register?error=Form+tidak+valid", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")
	redirect := r.FormValue("redirect")
	if redirect == "" || !strings.HasPrefix(redirect, "/") {
		redirect = "/account"
	}

	_, token, err := h.userSvc.Register(email, name, password, confirm, h.sessionAge)
	if err != nil {
		redirURL := fmt.Sprintf("/register?error=%s", strings.ReplaceAll(err.Error(), " ", "+"))
		if redirect != "/account" {
			redirURL += "&redirect=" + redirect
		}
		http.Redirect(w, r, redirURL, http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    token,
		Path:     "/",
		MaxAge:   h.sessionAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, redirect+"?msg=Pendaftaran+berhasil!+Selamat+datang+di+Azeotopup", http.StatusSeeOther)
}

func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("user_session"); err == nil && cookie.Value != "" {
		h.userSvc.Logout(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *UserHandler) AccountPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login?redirect=/account", http.StatusSeeOther)
		return
	}

	// Fetch fresh balance
	if refreshed, err := h.store.GetUserByID(user.ID); err == nil && refreshed != nil {
		user = refreshed
	}

	discountPercent := h.store.GetAZcoinDiscountPercent()
	orders, _ := h.store.GetOrdersByUserID(user.ID, 50)
	txs, _ := h.store.GetAZcoinTransactions(user.ID, 50)

	data := map[string]interface{}{
		"Title":           "Akun Saya & Saldo AZcoin — Azeotopup",
		"User":            user,
		"DiscountPercent": discountPercent,
		"Orders":          orders,
		"Transactions":    txs,
		"Message":         r.URL.Query().Get("msg"),
		"Error":           r.URL.Query().Get("error"),
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/user_account.html",
	}, data)
}

func (h *UserHandler) RedeemVoucherSubmit(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/account?error=Form+tidak+valid", http.StatusSeeOther)
		return
	}

	code := r.FormValue("voucher_code")
	amount, newBalance, err := h.userSvc.RedeemVoucher(user.ID, code)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/account?error=%s", strings.ReplaceAll(err.Error(), " ", "+")), http.StatusSeeOther)
		return
	}

	msg := fmt.Sprintf("Berhasil!+Voucher+%s+berhasil+di-redeem.+Saldo+bertambah+%d+AZcoin+(Total:+%d)", code, amount, newBalance)
	http.Redirect(w, r, "/account?msg="+msg, http.StatusSeeOther)
}

// API for fast AJAX redeem on Account Page
func (h *UserHandler) APIRedeemVoucher(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := middleware.GetUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Silakan login terlebih dahulu"})
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Format request tidak valid"})
		return
	}

	amount, newBalance, err := h.userSvc.RedeemVoucher(user.ID, req.Code)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: true,
		Message: fmt.Sprintf("Voucher berhasil di-redeem! +%d AZcoin", amount),
		Data: map[string]interface{}{
			"amount":      amount,
			"new_balance": newBalance,
		},
	})
}
