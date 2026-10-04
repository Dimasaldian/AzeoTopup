package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"topupku/handler"
	"topupku/middleware"
	"topupku/model"
	"topupku/store"
)

type ProductAdminHandler struct {
	store *store.SQLiteStore
}

func NewProductAdminHandler(st *store.SQLiteStore) *ProductAdminHandler {
	return &ProductAdminHandler{store: st}
}

func (h *ProductAdminHandler) ProductList(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	gameIDStr := r.PathValue("id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=ID+game+tidak+valid", http.StatusSeeOther)
		return
	}

	game, err := h.store.GetGameByID(gameID)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=Game+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	products, err := h.store.GetProductsByGameID(gameID, false)
	if err != nil {
		products = nil
	}

	currentMarginType := "percent"
	currentMarginValue := 6
	for _, p := range products {
		if p.MarginType == "percent" || p.MarginType == "fixed" {
			currentMarginType = p.MarginType
			currentMarginValue = p.MarginValue
			break
		}
	}

	data := map[string]interface{}{
		"Title":              "Produk " + game.Name + " — Azeotopup Admin",
		"ActiveTab":          "games",
		"AdminUser":          adminUser,
		"Game":               game,
		"Products":           products,
		"CurrentMarginType":  currentMarginType,
		"CurrentMarginValue": currentMarginValue,
		"Message":            r.URL.Query().Get("msg"),
		"Error":              r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/products.html",
	}, data)
}

func (h *ProductAdminHandler) ProductForm(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=ID+produk+tidak+valid", http.StatusSeeOther)
		return
	}

	product, err := h.store.GetProductByID(id)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=Produk+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	game, _ := h.store.GetGameByID(product.GameID)
	groups, _ := h.store.GetProductGroupsByGameID(product.GameID)

	data := map[string]interface{}{
		"Title":     "Edit Produk — Azeotopup Admin",
		"ActiveTab": "games",
		"AdminUser": adminUser,
		"Product":   product,
		"Game":      game,
		"Groups":    groups,
		"Error":     r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/product_form.html",
	}, data)
}

func (h *ProductAdminHandler) ProductUpdate(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/products/%d/edit?error=Form+tidak+valid", id), http.StatusSeeOther)
		return
	}

	prod, err := h.store.GetProductByID(id)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=Produk+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	customSKU := strings.TrimSpace(r.FormValue("custom_sku"))
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	marginType := r.FormValue("margin_type")
	marginVal, _ := strconv.Atoi(r.FormValue("margin_value"))
	manualSell, _ := strconv.Atoi(r.FormValue("manual_sell_price"))
	iconEmoji := strings.TrimSpace(r.FormValue("icon_emoji"))
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	isActive := r.FormValue("is_active") == "1" || r.FormValue("is_active") == "on"
	isPopular := r.FormValue("is_popular") == "1" || r.FormValue("is_popular") == "on"

	if displayName == "" {
		displayName = prod.DigiflazzName
	}
	if iconEmoji == "" {
		iconEmoji = "💎"
	}

	// Calculate sell price
	sellPrice := prod.CostPrice
	if marginType == "manual" {
		if manualSell > 0 {
			sellPrice = manualSell
		} else {
			sellPrice = prod.SellPrice
		}
	} else {
		sellPrice = model.CalculateSellPrice(prod.CostPrice, marginType, marginVal)
	}

	groupIDStr := strings.TrimSpace(r.FormValue("group_id"))
	newGroupName := strings.TrimSpace(r.FormValue("new_group_name"))
	if newGroupName != "" {
		gID, err := h.store.GetOrCreateProductGroup(prod.GameID, newGroupName)
		if err == nil && gID > 0 {
			prod.GroupID = &gID
		}
	} else if groupIDStr != "" {
		gID, err := strconv.ParseInt(groupIDStr, 10, 64)
		if err == nil && gID > 0 {
			prod.GroupID = &gID
		}
	}

	isPromo := r.FormValue("is_promo") == "1" || r.FormValue("is_promo") == "on"
	promoPrice, _ := strconv.Atoi(r.FormValue("promo_price"))
	promoQuota, _ := strconv.Atoi(r.FormValue("promo_quota"))
	promoRemainingStr := strings.TrimSpace(r.FormValue("promo_remaining"))
	promoRemaining, err := strconv.Atoi(promoRemainingStr)
	if err != nil || promoRemainingStr == "" {
		promoRemaining = promoQuota
	}

	digiflazzSKU := strings.TrimSpace(r.FormValue("digiflazz_sku"))
	if digiflazzSKU != "" {
		prod.DigiflazzSKU = digiflazzSKU
	}

	prod.CustomSKU = customSKU
	prod.DisplayName = displayName
	prod.MarginType = marginType
	prod.MarginValue = marginVal
	prod.SellPrice = sellPrice
	prod.IconEmoji = iconEmoji
	prod.SortOrder = sortOrder
	prod.IsActive = isActive
	prod.IsPopular = isPopular
	prod.IsPromo = isPromo
	prod.PromoPrice = promoPrice
	prod.PromoQuota = promoQuota
	prod.PromoRemaining = promoRemaining

	if err := h.store.UpdateProduct(prod); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/products/%d/edit?error=%s", id, err.Error()), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "product.update", "product", fmt.Sprintf("%d", id), "", fmt.Sprintf("Updated product %s", displayName))

	http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?msg=Produk+berhasil+diupdate!", prod.GameID), http.StatusSeeOther)
}

func (h *ProductAdminHandler) ProductToggle(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	prod, err := h.store.GetProductByID(id)
	if err != nil {
		http.Redirect(w, r, "/admin/games", http.StatusSeeOther)
		return
	}

	_ = h.store.ToggleProduct(id)
	_ = h.store.CreateAuditLog(adminUser.ID, "product.toggle", "product", idStr, "", "Toggled product status")

	http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?msg=Status+produk+berhasil+diubah", prod.GameID), http.StatusSeeOther)
}

func (h *ProductAdminHandler) ProductDelete(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	prod, err := h.store.GetProductByID(id)
	if err != nil {
		http.Redirect(w, r, "/admin/games", http.StatusSeeOther)
		return
	}

	gameID := prod.GameID
	_ = h.store.DeleteProduct(id)
	_ = h.store.CreateAuditLog(adminUser.ID, "product.delete", "product", idStr, "", "Deleted product")

	http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?msg=Produk+berhasil+dihapus", gameID), http.StatusSeeOther)
}

func (h *ProductAdminHandler) ProductBulkToggle(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	gameIDStr := r.FormValue("game_id")
	action := r.FormValue("action") // enable or disable
	gameID, _ := strconv.ParseInt(gameIDStr, 10, 64)

	active := action == "enable"
	_ = h.store.BulkToggleProducts(gameID, active)
	_ = h.store.CreateAuditLog(adminUser.ID, "product.bulk_toggle", "game", gameIDStr, "", fmt.Sprintf("Bulk toggle products to active=%v", active))

	http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?msg=Bulk+status+berhasil+diperbarui", gameID), http.StatusSeeOther)
}

func (h *ProductAdminHandler) ProductBulkMargin(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	gameIDStr := r.FormValue("game_id")
	marginType := r.FormValue("margin_type")
	marginVal, _ := strconv.Atoi(r.FormValue("margin_value"))
	gameID, _ := strconv.ParseInt(gameIDStr, 10, 64)

	_ = h.store.BulkMarginProducts(gameID, marginType, marginVal)
	_ = h.store.CreateAuditLog(adminUser.ID, "product.bulk_margin", "game", gameIDStr, "", fmt.Sprintf("Bulk set margin %s=%d", marginType, marginVal))

	http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?msg=Bulk+margin+berhasil+diterapkan!", gameID), http.StatusSeeOther)
}
