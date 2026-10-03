package admin

import (
	"net/http"

	"topupku/handler"
	"topupku/middleware"
	"topupku/service"
	"topupku/store"
)

type DashboardHandler struct {
	store     *store.SQLiteStore
	digiflazz *service.DigiflazzClient
}

func NewDashboardHandler(st *store.SQLiteStore, df *service.DigiflazzClient) *DashboardHandler {
	return &DashboardHandler{
		store:     st,
		digiflazz: df,
	}
}

func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	stats, err := h.store.GetDashboardStats()
	if err != nil {
		http.Error(w, "Gagal memuat statistik", http.StatusInternalServerError)
		return
	}

	errMsg := r.URL.Query().Get("error")
	msg := r.URL.Query().Get("msg")

	balance, err := h.digiflazz.CheckBalance()
	if err != nil {
		if errMsg == "" {
			errMsg = "Koneksi Digiflazz: " + err.Error()
		}
	} else {
		stats.DigiflazzBalance = balance
	}

	recentOrders, err := h.store.GetRecentOrders(10)
	if err != nil {
		recentOrders = nil
	}

	data := map[string]interface{}{
		"Title":        "Dashboard Admin — Azeotopup",
		"ActiveTab":    "dashboard",
		"AdminUser":    adminUser,
		"Stats":        stats,
		"RecentOrders": recentOrders,
		"Error":        errMsg,
		"Message":      msg,
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/dashboard.html",
	}, data)
}
