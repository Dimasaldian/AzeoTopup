package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"topupku/handler"
	"topupku/middleware"
	"topupku/service"
	"topupku/store"
)

type OrderAdminHandler struct {
	store    *store.SQLiteStore
	orderSvc *service.OrderService
}

func NewOrderAdminHandler(st *store.SQLiteStore, os *service.OrderService) *OrderAdminHandler {
	return &OrderAdminHandler{
		store:    st,
		orderSvc: os,
	}
}

func (h *OrderAdminHandler) OrderList(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	statusFilter := r.URL.Query().Get("status")
	gameFilter := r.URL.Query().Get("game")
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 25
	offset := (page - 1) * limit

	orders, total, err := h.store.GetAllOrders(statusFilter, gameFilter, search, limit, offset)
	if err != nil {
		orders = nil
		total = 0
	}

	games, _ := h.store.GetAllGames(false)
	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	data := map[string]interface{}{
		"Title":        "Daftar Pesanan — Azeotopup Admin",
		"ActiveTab":    "orders",
		"AdminUser":    adminUser,
		"Orders":       orders,
		"Games":        games,
		"StatusFilter": statusFilter,
		"GameFilter":   gameFilter,
		"Search":       search,
		"Page":         page,
		"TotalPages":   totalPages,
		"Total":        total,
		"Message":      r.URL.Query().Get("msg"),
		"Error":        r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/orders.html",
	}, data)
}

func (h *OrderAdminHandler) OrderDetail(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	orderID := r.PathValue("id")
	order, err := h.store.GetOrderByID(orderID)
	if err != nil {
		http.Redirect(w, r, "/admin/orders?error=Pesanan+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	product, _ := h.store.GetProductByID(order.ProductID)

	data := map[string]interface{}{
		"Title":     "Detail Pesanan " + order.ID + " — Azeotopup Admin",
		"ActiveTab": "orders",
		"AdminUser": adminUser,
		"Order":     order,
		"Product":   product,
		"Message":   r.URL.Query().Get("msg"),
		"Error":     r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/order_detail.html",
	}, data)
}

func (h *OrderAdminHandler) OrderRetry(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	orderID := r.PathValue("id")
	if err := h.orderSvc.RetryTopUp(orderID); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/orders/%s?error=Gagal+retry:+%s", orderID, err.Error()), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "order.retry", "order", orderID, "", "Manual retry topup to Digiflazz")

	http.Redirect(w, r, fmt.Sprintf("/admin/orders/%s?msg=Top-up+berhasil+dikirim+ulang!", orderID), http.StatusSeeOther)
}

func (h *OrderAdminHandler) OrderRefund(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	orderID := r.PathValue("id")
	note := r.FormValue("note")
	if note == "" {
		note = "Refund diproses manual oleh admin"
	}

	_ = h.orderSvc.RefundOrder(orderID, note)
	_ = h.store.CreateAuditLog(adminUser.ID, "order.refund", "order", orderID, "", "Marked as refund: "+note)

	http.Redirect(w, r, fmt.Sprintf("/admin/orders/%s?msg=Status+diubah+menjadi+Refund", orderID), http.StatusSeeOther)
}

func (h *OrderAdminHandler) OrderNote(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	orderID := r.PathValue("id")
	note := r.FormValue("note")

	_ = h.store.UpdateOrderNote(orderID, note, "")
	_ = h.store.CreateAuditLog(adminUser.ID, "order.note", "order", orderID, "", "Updated admin note")

	http.Redirect(w, r, fmt.Sprintf("/admin/orders/%s?msg=Catatan+berhasil+disimpan", orderID), http.StatusSeeOther)
}
