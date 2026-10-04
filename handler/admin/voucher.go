package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"topupku/handler"
	"topupku/middleware"
	"topupku/model"
	"topupku/service"
	"topupku/store"
)

type VoucherAdminHandler struct {
	store   *store.SQLiteStore
	userSvc *service.UserService
}

func NewVoucherAdminHandler(st *store.SQLiteStore, us *service.UserService) *VoucherAdminHandler {
	return &VoucherAdminHandler{
		store:   st,
		userSvc: us,
	}
}

func (h *VoucherAdminHandler) VoucherList(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	statusFilter := r.URL.Query().Get("status") // "all", "unused", "used"
	vouchers, err := h.store.GetVouchers(statusFilter, 100)
	if err != nil {
		vouchers = nil
	}

	stats := h.store.GetVoucherStats()
	discountPercent := h.store.GetAZcoinDiscountPercent()

	data := map[string]interface{}{
		"Title":           "Manajemen Voucher & AZcoin — Azeotopup Admin",
		"ActiveTab":       "vouchers",
		"AdminUser":       adminUser,
		"Vouchers":        vouchers,
		"Stats":           stats,
		"DiscountPercent": discountPercent,
		"StatusFilter":    statusFilter,
		"Message":         r.URL.Query().Get("msg"),
		"Error":           r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/vouchers.html",
	}, data)
}

func (h *VoucherAdminHandler) VoucherGenerate(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/vouchers?error=Form+tidak+valid", http.StatusSeeOther)
		return
	}

	amount, _ := strconv.ParseInt(r.FormValue("amount"), 10, 64)
	qty, _ := strconv.Atoi(r.FormValue("qty"))
	note := strings.TrimSpace(r.FormValue("note"))

	codes, err := h.userSvc.GenerateVouchers(amount, qty, note)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/vouchers?error=%s", strings.ReplaceAll(err.Error(), " ", "+")), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "voucher.generate", "voucher", fmt.Sprintf("%d", len(codes)), "",
		fmt.Sprintf("Generated %d vouchers @ %d AZcoin", len(codes), amount))

	msg := fmt.Sprintf("Berhasil+membuat+%d+voucher+AZcoin+nominal+Rp+%d!", len(codes), amount)
	http.Redirect(w, r, "/admin/vouchers?msg="+msg, http.StatusSeeOther)
}

func (h *VoucherAdminHandler) VoucherDelete(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.store.DeleteUnusedVoucher(id); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/vouchers?error=%s", strings.ReplaceAll(err.Error(), " ", "+")), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "voucher.delete", "voucher", fmt.Sprintf("%d", id), "", "Deleted unused voucher")

	http.Redirect(w, r, "/admin/vouchers?msg=Voucher+berhasil+dihapus", http.StatusSeeOther)
}

func (h *VoucherAdminHandler) UpdateDiscount(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	pctStr := strings.TrimSpace(r.FormValue("discount_percent"))
	pct, err := strconv.Atoi(pctStr)
	if err != nil || pct < 0 || pct > 50 {
		http.Redirect(w, r, "/admin/vouchers?error=Diskon+harus+antara+0+-+50%%", http.StatusSeeOther)
		return
	}

	oldVal := h.store.GetSetting(model.SettingAZcoinDiscountPercent, "3")
	_ = h.store.SetSetting(model.SettingAZcoinDiscountPercent, strconv.Itoa(pct))
	_ = h.store.CreateAuditLog(adminUser.ID, "setting.azcoin_discount", "setting", model.SettingAZcoinDiscountPercent, oldVal, strconv.Itoa(pct))

	http.Redirect(w, r, fmt.Sprintf("/admin/vouchers?msg=Diskon+AZcoin+berhasil+diubah+menjadi+%d%%", pct), http.StatusSeeOther)
}
