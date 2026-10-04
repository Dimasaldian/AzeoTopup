package handler

import (
	"fmt"
	"net/http"
	"strings"

	"topupku/middleware"
	"topupku/model"
	"topupku/store"
)

type TicketHandler struct {
	store *store.SQLiteStore
}

func NewTicketHandler(st *store.SQLiteStore) *TicketHandler {
	return &TicketHandler{
		store: st,
	}
}

func (h *TicketHandler) PageSupport(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	orderID := strings.TrimSpace(r.URL.Query().Get("order_id"))

	data := map[string]interface{}{
		"Title":   "Bantuan & Pusat Aduan — Azeotopup",
		"User":    user,
		"OrderID": orderID,
		"Message": r.URL.Query().Get("msg"),
		"Error":   r.URL.Query().Get("error"),
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/support.html",
	}, data)
}

func (h *TicketHandler) SubmitTicket(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/bantuan?error=Permintaan+tidak+valid", http.StatusSeeOther)
		return
	}

	customerName := strings.TrimSpace(r.FormValue("customer_name"))
	customerContact := strings.TrimSpace(r.FormValue("customer_contact"))
	category := strings.TrimSpace(r.FormValue("category"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	description := strings.TrimSpace(r.FormValue("description"))
	orderID := strings.TrimSpace(r.FormValue("order_id"))

	if customerName == "" || customerContact == "" || subject == "" || description == "" {
		http.Redirect(w, r, fmt.Sprintf("/bantuan?order_id=%s&error=Harap+lengkapi+semua+kolom+wajib", orderID), http.StatusSeeOther)
		return
	}

	ticket := &model.SupportTicket{
		ID:              store.GenerateTicketID(),
		OrderID:         orderID,
		CustomerName:    customerName,
		CustomerContact: customerContact,
		Category:        category,
		Subject:         subject,
		Description:     description,
		Status:          "open",
	}

	if err := h.store.CreateTicket(ticket); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/bantuan?order_id=%s&error=Gagal+mengirim+aduan:+%s", orderID, err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/bantuan/"+ticket.ID, http.StatusSeeOther)
}

func (h *TicketHandler) PageTicketDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	id := r.PathValue("id")

	ticket, err := h.store.GetTicketByID(id)
	if err != nil || ticket == nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]interface{}{
		"Title":  fmt.Sprintf("Tiket Aduan %s — Azeotopup", ticket.ID),
		"User":   user,
		"Ticket": ticket,
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/support_detail.html",
	}, data)
}
