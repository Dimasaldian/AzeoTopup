package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"topupku/handler"
	"topupku/middleware"
	"topupku/store"
)

type AdminUserHandler struct {
	store *store.SQLiteStore
}

func NewAdminUserHandler(st *store.SQLiteStore) *AdminUserHandler {
	return &AdminUserHandler{
		store: st,
	}
}

func (h *AdminUserHandler) List(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	admins, err := h.store.GetAllAdmins()
	if err != nil {
		admins = nil
	}

	data := map[string]interface{}{
		"Title":     "Manajemen Akun Admin — Azeotopup Console",
		"ActiveTab": "admins",
		"AdminUser": adminUser,
		"Admins":    admins,
		"Message":   r.URL.Query().Get("msg"),
		"Error":     r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/admins.html",
	}, data)
}

func (h *AdminUserHandler) Create(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/admins?error=Permintaan+tidak+valid", http.StatusSeeOther)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	password := strings.TrimSpace(r.FormValue("password"))
	role := strings.TrimSpace(r.FormValue("role"))

	if username == "" || password == "" || displayName == "" {
		http.Redirect(w, r, "/admin/admins?error=Username,+Nama,+dan+Password+wajib+diisi", http.StatusSeeOther)
		return
	}

	if len(password) < 6 {
		http.Redirect(w, r, "/admin/admins?error=Password+minimal+6+karakter", http.StatusSeeOther)
		return
	}

	if role != "superadmin" {
		role = "admin"
	}

	// Check existing username
	existing, _ := h.store.GetAdminByUsername(username)
	if existing != nil {
		http.Redirect(w, r, "/admin/admins?error=Username+sudah+digunakan", http.StatusSeeOther)
		return
	}

	created, err := h.store.CreateAdmin(username, password, displayName, role)
	if err != nil {
		http.Redirect(w, r, "/admin/admins?error=Gagal+membuat+admin:+"+err.Error(), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "CREATE", "admin_user", fmt.Sprintf("%d", created.ID), "", fmt.Sprintf("Created %s (%s)", username, role))

	http.Redirect(w, r, "/admin/admins?msg=Akun+admin+berhasil+dibuat", http.StatusSeeOther)
}

func (h *AdminUserHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/admins?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}

	if id == adminUser.ID {
		http.Redirect(w, r, "/admin/admins?error=Anda+tidak+dapat+menonaktifkan+akun+Anda+sendiri", http.StatusSeeOther)
		return
	}

	if err := h.store.ToggleAdminActive(id); err != nil {
		http.Redirect(w, r, "/admin/admins?error=Gagal+mengubah+status+admin", http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "TOGGLE", "admin_user", idStr, "", "Toggled active state")

	http.Redirect(w, r, "/admin/admins?msg=Status+admin+berhasil+diperbarui", http.StatusSeeOther)
}

func (h *AdminUserHandler) Update(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/admins?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/admins?error=Permintaan+tidak+valid", http.StatusSeeOther)
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	role := strings.TrimSpace(r.FormValue("role"))
	newPassword := strings.TrimSpace(r.FormValue("new_password"))
	isActive := r.FormValue("is_active") == "1"

	if displayName == "" {
		http.Redirect(w, r, "/admin/admins?error=Nama+tampilan+wajib+diisi", http.StatusSeeOther)
		return
	}

	if id == adminUser.ID {
		// Prevent superadmin from demoting themselves or deactivating themselves
		role = "superadmin"
		isActive = true
	}

	if role != "superadmin" {
		role = "admin"
	}

	if newPassword != "" && len(newPassword) < 6 {
		http.Redirect(w, r, "/admin/admins?error=Password+baru+minimal+6+karakter", http.StatusSeeOther)
		return
	}

	if err := h.store.UpdateAdmin(id, displayName, role, isActive, newPassword); err != nil {
		http.Redirect(w, r, "/admin/admins?error=Gagal+memperbarui+admin:+"+err.Error(), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "UPDATE", "admin_user", idStr, "", fmt.Sprintf("Updated %s (%s)", displayName, role))

	http.Redirect(w, r, "/admin/admins?msg=Akun+admin+berhasil+diperbarui", http.StatusSeeOther)
}

func (h *AdminUserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/admins?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}

	if id == adminUser.ID {
		http.Redirect(w, r, "/admin/admins?error=Anda+tidak+dapat+menghapus+akun+Anda+sendiri", http.StatusSeeOther)
		return
	}

	if err := h.store.DeleteAdmin(id); err != nil {
		http.Redirect(w, r, "/admin/admins?error=Gagal+menghapus+admin", http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "DELETE", "admin_user", idStr, "", "Deleted admin user")

	http.Redirect(w, r, "/admin/admins?msg=Akun+admin+berhasil+dihapus", http.StatusSeeOther)
}
