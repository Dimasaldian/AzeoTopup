package handler

import (
	"net/http"

	"topupku/model"
	"topupku/service"
	"topupku/store"
)

type PageHandler struct {
	store     *store.SQLiteStore
	orders    *service.OrderService
	isDevMode bool
}

func NewPageHandler(st *store.SQLiteStore, os *service.OrderService, isDev bool) *PageHandler {
	return &PageHandler{
		store:     st,
		orders:    os,
		isDevMode: isDev,
	}
}

func (h *PageHandler) PageHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	games, err := h.store.GetAllGames(true)
	if err != nil {
		http.Error(w, "Gagal memuat game", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title": "Azeotopup — Top Up Game Termurah & Instan 24 Jam",
		"Games": games,
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/home.html",
	}, data)
}

func (h *PageHandler) PageGame(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	game, err := h.store.GetGameByCode(code)
	if err != nil || !game.IsActive {
		http.NotFound(w, r)
		return
	}

	products, err := h.store.GetProductsByGameID(game.ID, true)
	if err != nil {
		http.Error(w, "Gagal memuat produk", http.StatusInternalServerError)
		return
	}

	// Group products
	groupedProducts := make(map[string][]*model.Product)
	var groupNames []string
	for _, p := range products {
		gName := p.GroupName
		if gName == "" {
			gName = store.DetermineProductGroupName(game.Name, p.EffectiveName())
		}
		if _, exists := groupedProducts[gName]; !exists {
			groupNames = append(groupNames, gName)
		}
		groupedProducts[gName] = append(groupedProducts[gName], p)
	}

	// Filter promo products for the flash sale showcase
	var promoProducts []*model.Product
	for _, p := range products {
		if p.HasActivePromo() {
			promoProducts = append(promoProducts, p)
		}
	}

	data := map[string]interface{}{
		"Title":           game.Name + " — Top Up Instan | Azeotopup",
		"Game":            game,
		"Products":        products,
		"PromoProducts":   promoProducts,
		"GroupedProducts": groupedProducts,
		"GroupNames":      groupNames,
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/game.html",
	}, data)
}

func (h *PageHandler) PageOrderStatus(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("id")
	if orderID == "" {
		http.NotFound(w, r)
		return
	}

	order, err := h.orders.GetOrderByIDWithLiveCheck(orderID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]interface{}{
		"Title":     "Pesanan " + order.ID + " | Azeotopup",
		"Order":     order,
		"IsDevMode": h.isDevMode,
	}

	RenderTemplate(w, "template/layout.html", []string{
		"template/components/header.html",
		"template/components/footer.html",
		"template/status.html",
	}, data)
}
