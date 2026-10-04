package admin

import (
	"fmt"
	"net/http"
	"strings"

	"topupku/handler"
	"topupku/middleware"
	"topupku/store"
)

type TicketAdminHandler struct {
	store *store.SQLiteStore
}

func NewTicketAdminHandler(st *store.SQLiteStore) *TicketAdminHandler {
	return &TicketAdminHandler{
		store: st,
	}
}

func (h *TicketAdminHandler) TicketList(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	statusFilter := r.URL.Query().Get("status") // "open", "in_progress", "resolved", "all"
	if statusFilter == "" {
		statusFilter = "open"
	}

	tickets, err := h.store.GetTickets(statusFilter, 100)
	if err != nil {
		tickets = nil
	}

	openCount, inProgressCount, resolvedCount := h.store.GetTicketStats()

	data := map[string]interface{}{
		"Title":           "Pusat Aduan Pelanggan & CS — Azeotopup Console",
		"ActiveTab":       "tickets",
		"AdminUser":       adminUser,
		"Tickets":         tickets,
		"StatusFilter":    statusFilter,
		"OpenCount":       openCount,
		"InProgressCount": inProgressCount,
		"ResolvedCount":   resolvedCount,
		"Message":         r.URL.Query().Get("msg"),
		"Error":           r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/tickets.html",
	}, data)
}

func (h *TicketAdminHandler) TicketDetail(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)
	id := r.PathValue("id")

	ticket, err := h.store.GetTicketByID(id)
	if err != nil || ticket == nil {
		http.NotFound(w, r)
		return
	}

	// Related order if any
	var order interface{}
	if ticket.OrderID != "" {
		ord, _ := h.store.GetOrderByID(ticket.OrderID)
		order = ord
	}

	data := map[string]interface{}{
		"Title":     fmt.Sprintf("Detail Aduan %s — Azeotopup Console", ticket.ID),
		"ActiveTab": "tickets",
		"AdminUser": adminUser,
		"Ticket":    ticket,
		"Order":     order,
		"Message":   r.URL.Query().Get("msg"),
		"Error":     r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/ticket_detail.html",
	}, data)
}

func (h *TicketAdminHandler) TicketUpdate(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/tickets/%s?error=Permintaan+tidak+valid", id), http.StatusSeeOther)
		return
	}

	status := strings.TrimSpace(r.FormValue("status"))
	reply := strings.TrimSpace(r.FormValue("admin_reply"))

	if status == "" {
		status = "in_progress"
	}

	if err := h.store.UpdateTicket(id, status, reply, adminUser.ID); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/tickets/%s?error=Gagal+memperbarui+tiket", id), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "REPLY_TICKET", "support_ticket", id, "", fmt.Sprintf("Updated ticket to %s", status))

	http.Redirect(w, r, fmt.Sprintf("/admin/tickets/%s?msg=Tanggapan+dan+status+tiket+berhasil+disimpan", id), http.StatusSeeOther)
}
