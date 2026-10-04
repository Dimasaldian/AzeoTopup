package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"topupku/handler"
	"topupku/middleware"
	"topupku/service"
	"topupku/store"
)

type SyncAdminHandler struct {
	store     *store.SQLiteStore
	adminSvc  *service.AdminService
	digiflazz *service.DigiflazzClient
}

func NewSyncAdminHandler(st *store.SQLiteStore, as *service.AdminService, df *service.DigiflazzClient) *SyncAdminHandler {
	return &SyncAdminHandler{
		store:     st,
		adminSvc:  as,
		digiflazz: df,
	}
}

type ImportItemView struct {
	BuyerSKUCode   string
	ProductName    string
	Price          int
	IsImported     bool
	ExistingCustom string
	StockStatus    string
}

func (h *SyncAdminHandler) ProductImport(w http.ResponseWriter, r *http.Request) {
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

	// Invalidate cache to ensure fresh sellers and prices from Digiflazz
	h.digiflazz.InvalidateCache()
	dfItems, err := h.digiflazz.GetPriceList(game.Brand)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?error=%s", gameID, url.QueryEscape("Gagal mengambil data Digiflazz: "+err.Error())), http.StatusSeeOther)
		return
	}

	// Get existing products in DB
	existingProds, _ := h.store.GetProductsByGameID(gameID, false)
	existingMap := make(map[string]string)
	for _, p := range existingProds {
		existingMap[p.DigiflazzSKU] = p.CustomSKU
	}

	var importList []ImportItemView
	for _, it := range dfItems {
		custom, exists := existingMap[it.BuyerSKUCode]
		importList = append(importList, ImportItemView{
			BuyerSKUCode:   it.BuyerSKUCode,
			ProductName:    it.ProductName,
			Price:          it.Price,
			IsImported:     exists,
			ExistingCustom: custom,
			StockStatus:    "Tersedia",
		})
	}

	defaultMarginType := "percent"
	defaultMarginValue := 6
	for _, p := range existingProds {
		if p.MarginType == "percent" || p.MarginType == "fixed" {
			defaultMarginType = p.MarginType
			defaultMarginValue = p.MarginValue
			break
		}
	}

	data := map[string]interface{}{
		"Title":              "Import Produk Digiflazz — " + game.Name,
		"ActiveTab":          "games",
		"AdminUser":          adminUser,
		"Game":               game,
		"ImportList":         importList,
		"TotalFound":         len(importList),
		"DefaultMarginType":  defaultMarginType,
		"DefaultMarginValue": defaultMarginValue,
		"Error":              r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/product_import.html",
	}, data)
}

func (h *SyncAdminHandler) ProductImportExec(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	gameIDStr := r.PathValue("id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}

	game, err := h.store.GetGameByID(gameID)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=Game+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products/import?error=Form+tidak+valid", gameID), http.StatusSeeOther)
		return
	}

	selectedSKUs := r.Form["sku"]
	defaultMarginType := r.FormValue("default_margin_type")
	if defaultMarginType == "" {
		defaultMarginType = "fixed"
	}
	defaultMarginVal, _ := strconv.Atoi(r.FormValue("default_margin_value"))
	if defaultMarginVal <= 0 {
		defaultMarginVal = 1500
	}

	var itemsToImport []service.ImportProductItem

	for _, sku := range selectedSKUs {
		costPrice, _ := strconv.Atoi(r.FormValue("cost_" + sku))
		rawName := r.FormValue("name_" + sku)
		customSKU := strings.TrimSpace(r.FormValue("custom_sku_" + sku))
		cleanName := strings.TrimSpace(r.FormValue("display_name_" + sku))
		if cleanName == "" {
			cleanName = h.adminSvc.FormatCleanName(rawName, game.Brand)
		}
		if customSKU == "" {
			customSKU = fmt.Sprintf("TK-%s-%s", strings.ToUpper(game.Code), strings.ToUpper(sku))
		}

		itemsToImport = append(itemsToImport, service.ImportProductItem{
			BuyerSKUCode: sku,
			ProductName:  rawName,
			CostPrice:    costPrice,
			CustomSKU:    customSKU,
			DisplayName:  cleanName,
			MarginType:   defaultMarginType,
			MarginValue:  defaultMarginVal,
			IconEmoji:    "💎",
		})
	}

	importedCount, err := h.adminSvc.ImportProducts(gameID, itemsToImport, adminUser.ID)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products/import?error=%s", gameID, err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?msg=Berhasil+mengimport+%d+produk!", gameID, importedCount), http.StatusSeeOther)
}

func (h *SyncAdminHandler) SyncPriceList(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	count, err := h.adminSvc.SyncPriceList(adminUser.ID)
	if err != nil {
		http.Redirect(w, r, "/admin?error="+url.QueryEscape("Sync gagal: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin?msg=Berhasil+sinkronisasi+%d+produk+dari+Digiflazz!", count), http.StatusSeeOther)
}

func (h *SyncAdminHandler) CheckBalance(w http.ResponseWriter, r *http.Request) {
	balance, err := h.digiflazz.CheckBalance()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "balance": balance})
}
