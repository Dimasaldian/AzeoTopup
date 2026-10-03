package admin

import (
	"net/http"

	"topupku/handler"
	"topupku/middleware"
	"topupku/store"
)

type AuditAdminHandler struct {
	store *store.SQLiteStore
}

func NewAuditAdminHandler(st *store.SQLiteStore) *AuditAdminHandler {
	return &AuditAdminHandler{store: st}
}

func (h *AuditAdminHandler) AuditLog(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	logs, err := h.store.GetAuditLogs(100)
	if err != nil {
		logs = nil
	}

	data := map[string]interface{}{
		"Title":     "Audit Log Aktivitas — Azeotopup Admin",
		"ActiveTab": "audit",
		"AdminUser": adminUser,
		"Logs":      logs,
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/audit_log.html",
	}, data)
}
