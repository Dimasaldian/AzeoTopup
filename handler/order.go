package handler

import (
	"encoding/json"
	"net/http"

	"topupku/middleware"
	"topupku/model"
	"topupku/service"
	"topupku/store"
)

type OrderHandler struct {
	store    *store.SQLiteStore
	orders   *service.OrderService
	apigames *service.ApiGamesClient
}

func NewOrderHandler(st *store.SQLiteStore, os *service.OrderService, ag *service.ApiGamesClient) *OrderHandler {
	return &OrderHandler{
		store:    st,
		orders:   os,
		apigames: ag,
	}
}

type CreateOrderRequest struct {
	GameCode      string `json:"game_code"`
	CustomerNo    string `json:"customer_no"`
	CustomerNo2   string `json:"customer_no2"`
	CustomerEmail string `json:"customer_email"`
	ProductID     int64  `json:"product_id"`
	PaymentMethod string `json:"payment_method"` // "qris" or "azcoin"
}

type JSONResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Format data tidak valid"})
		return
	}

	user := middleware.GetUser(r)

	// If guest didn't select, default to QRIS
	if req.PaymentMethod == "" {
		req.PaymentMethod = model.PaymentMethodQRIS
	}

	// Auto-fill email if user is logged in and email is empty
	if req.CustomerEmail == "" && user != nil {
		req.CustomerEmail = user.Email
	}

	order, err := h.orders.CreateOrderFor(req.GameCode, req.CustomerNo, req.CustomerNo2, req.CustomerEmail, req.ProductID, user, req.PaymentMethod)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	respData := map[string]interface{}{
		"order_id":       order.ID,
		"redirect_url":   "/order/" + order.ID,
		"price":          order.Price,
		"payment_method": order.PaymentMethod,
		"status":         order.Status,
	}
	if order.PaymentMethod == model.PaymentMethodQRIS {
		respData["qr_url"] = order.PaymentQRURL
		respData["checkout_url"] = order.PaymentCheckoutURL
	}

	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: true,
		Data:    respData,
	})
}

func (h *OrderHandler) GetOrderStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	orderID := r.PathValue("id")
	if orderID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Order ID wajib diisi"})
		return
	}

	order, err := h.orders.GetOrderByIDWithLiveCheck(orderID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Pesanan tidak ditemukan"})
		return
	}

	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"id":          order.ID,
			"status":      order.Status,
			"status_text": order.StatusBadge(),
			"sn":          order.DigiflazzSN,
			"message":     order.DigiflazzMessage,
		},
	})
}

func (h *OrderHandler) SimulatePayment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	orderID := r.PathValue("id")
	order, err := h.store.GetOrderByID(orderID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Pesanan tidak ditemukan"})
		return
	}

	if err := h.orders.ProcessPaymentReceived(order.PaymentTrxID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: true,
		Message: "Simulasi pembayaran berhasil! Top-up sedang diproses.",
	})
}

func (h *OrderHandler) CheckUsername(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	game := r.URL.Query().Get("game")
	userID := r.URL.Query().Get("user_id")
	zoneID := r.URL.Query().Get("zone_id")

	if game == "" || userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONResponse{
			Success: false,
			Message: "Parameter game dan user_id wajib diisi",
		})
		return
	}

	result, err := h.apigames.CheckUsername(game, userID, zoneID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(JSONResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: result.Success,
		Message: result.Message,
		Data: map[string]interface{}{
			"username": result.Username,
		},
	})
}

