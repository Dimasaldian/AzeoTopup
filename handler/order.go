package handler

import (
	"encoding/json"
	"net/http"

	"topupku/service"
	"topupku/store"
)

type OrderHandler struct {
	store  *store.SQLiteStore
	orders *service.OrderService
}

func NewOrderHandler(st *store.SQLiteStore, os *service.OrderService) *OrderHandler {
	return &OrderHandler{
		store:  st,
		orders: os,
	}
}

type CreateOrderRequest struct {
	GameCode      string `json:"game_code"`
	CustomerNo    string `json:"customer_no"`
	CustomerNo2   string `json:"customer_no2"`
	CustomerEmail string `json:"customer_email"`
	ProductID     int64  `json:"product_id"`
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

	order, err := h.orders.CreateOrder(req.GameCode, req.CustomerNo, req.CustomerNo2, req.CustomerEmail, req.ProductID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"order_id":     order.ID,
			"redirect_url": "/order/" + order.ID,
			"qr_url":       order.PaymentQRURL,
			"checkout_url": order.PaymentCheckoutURL,
			"price":        order.Price,
		},
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
