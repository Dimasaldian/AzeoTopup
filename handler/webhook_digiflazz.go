package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"topupku/service"
)

type DigiflazzWebhookHandler struct {
	orders        *service.OrderService
	webhookSecret string
}

func NewDigiflazzWebhookHandler(os *service.OrderService, secret string) *DigiflazzWebhookHandler {
	return &DigiflazzWebhookHandler{
		orders:        os,
		webhookSecret: secret,
	}
}

type DigiflazzCallbackPayload struct {
	Data struct {
		RefID   string `json:"ref_id"`
		Status  string `json:"status"`
		RC      string `json:"rc"`
		SN      string `json:"sn"`
		Message string `json:"message"`
	} `json:"data"`
	Hook   string `json:"hook"`
	Secret string `json:"secret"`
}

func (h *DigiflazzWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload DigiflazzCallbackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[Webhook Digiflazz] Invalid JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if h.webhookSecret != "" && payload.Secret != "" && payload.Secret != h.webhookSecret {
		log.Printf("[Webhook Digiflazz] Secret mismatch: expected=%s got=%s", h.webhookSecret, payload.Secret)
		http.Error(w, "Unauthorized secret", http.StatusUnauthorized)
		return
	}

	log.Printf("[Webhook Digiflazz] Callback refID=%s status=%s sn=%s msg=%s",
		payload.Data.RefID, payload.Data.Status, payload.Data.SN, payload.Data.Message)

	if err := h.orders.ProcessDigiflazzWebhook(payload.Data.RefID, payload.Data.Status, payload.Data.SN, payload.Data.Message); err != nil {
		log.Printf("[Webhook Digiflazz] Error updating order: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
