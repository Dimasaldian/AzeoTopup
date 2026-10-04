package admin

import (
	"net/http"
	"strings"

	"topupku/handler"
	"topupku/middleware"
	"topupku/store"
)

type UserAdminHandler struct {
	store *store.SQLiteStore
}

func NewUserAdminHandler(st *store.SQLiteStore) *UserAdminHandler {
	return &UserAdminHandler{store: st}
}

func (h *UserAdminHandler) UserList(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	users, err := h.store.GetAllUsers(search, 100)
	if err != nil {
		users = nil
	}

	data := map[string]interface{}{
		"Title":     "Daftar Pengguna / Member — Azeotopup Admin",
		"ActiveTab": "users",
		"AdminUser": adminUser,
		"Users":     users,
		"Search":    search,
		"Message":   r.URL.Query().Get("msg"),
		"Error":     r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/users.html",
	}, data)
}
