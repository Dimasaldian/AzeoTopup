package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"topupku/service"
)

type AutoGoPayWebhookHandler struct {
	orders    *service.OrderService
	autogopay *service.AutoGoPayClient
}

func NewAutoGoPayWebhookHandler(os *service.OrderService, agp *service.AutoGoPayClient) *AutoGoPayWebhookHandler {
	return &AutoGoPayWebhookHandler{
		orders:    os,
		autogopay: agp,
	}
}

func (h *AutoGoPayWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	sig := r.Header.Get("X-Signature")
	if !h.autogopay.VerifySignature(body, sig) {
		log.Printf("[Webhook AutoGoPay] Invalid signature: %s", sig)
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	var payload service.AutoGoPayWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[Webhook AutoGoPay] Failed to unmarshal JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[Webhook AutoGoPay] Received event=%s trxID=%s status=%s",
		payload.Event, payload.Transaction.TransactionID, payload.Transaction.Status)

	if payload.Transaction.Status == "PAID" {
		if err := h.orders.ProcessPaymentReceived(payload.Transaction.TransactionID); err != nil {
			log.Printf("[Webhook AutoGoPay] Error processing payment: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
