package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AutoGoPayClient struct {
	APIKey  string
	BaseURL string
	client  *http.Client
}

func NewAutoGoPayClient(apiKey, baseURL string) *AutoGoPayClient {
	if baseURL == "" {
		baseURL = "https://v1-gateway.autogopay.site"
	}
	return &AutoGoPayClient{
		APIKey:  apiKey,
		BaseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *AutoGoPayClient) IsConfigured() bool {
	return a.APIKey != ""
}

type QRISData struct {
	TransactionID     string `json:"transaction_id"`
	OrderID           string `json:"order_id"`
	Amount            int    `json:"amount"`
	TransactionStatus string `json:"transaction_status"`
	QRString          string `json:"qr_string"`
	QRURL             string `json:"qr_url"`
	CheckoutURL       string `json:"checkout_url"`
	TransactionTime   string `json:"transaction_time"`
	ExpiryTime        string `json:"expiry_time"`
}

type QRISGenerateResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    QRISData `json:"data"`
}

func (a *AutoGoPayClient) GenerateQRIS(orderID string, amount int) (*QRISData, error) {
	if !a.IsConfigured() {
		// Mock / Simulation mode
		now := time.Now()
		expiry := now.Add(15 * time.Minute)
		trxID := fmt.Sprintf("AGP-SIM-%d", now.UnixNano())
		qrString := fmt.Sprintf("00020101021226610014COM.GO-JEK.WWW01189360091430000000000215%s51440014ID.LINKAJA.WWW011893600914300000000002155204581253033605802ID5911TOPUPKU DEV6007JAKARTA62070703A016304%d", trxID, amount)
		
		// Use a public QR code generator API as preview URL, or inline SVG
		qrURL := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=300x300&data=%s", qrString)
		
		return &QRISData{
			TransactionID:     trxID,
			OrderID:           fmt.Sprintf("AGP-%s", orderID),
			Amount:            amount,
			TransactionStatus: "pending",
			QRString:          qrString,
			QRURL:             qrURL,
			CheckoutURL:       fmt.Sprintf("/order/%s", orderID),
			TransactionTime:   now.Format("2006-01-02 15:04:05"),
			ExpiryTime:        expiry.Format("2006-01-02 15:04:05"),
		}, nil
	}

	payload := map[string]interface{}{
		"amount": amount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", a.BaseURL+"/qris/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+a.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("autogopay generate error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var res QRISGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if !res.Success {
		return nil, fmt.Errorf("autogopay error: %s", res.Message)
	}

	return &res.Data, nil
}

func (a *AutoGoPayClient) VerifySignature(rawBody []byte, signature string) bool {
	if a.APIKey == "" {
		return true // Allow simulated callbacks in dev mode
	}
	mac := hmac.New(sha256.New, []byte(a.APIKey))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

type AutoGoPayWebhookPayload struct {
	Event       string `json:"event"`
	Timestamp   string `json:"timestamp"`
	Transaction struct {
		TransactionID string `json:"transaction_id"`
		OrderID       string `json:"order_id"`
		Amount        int    `json:"amount"`
		Status        string `json:"status"` // PAID, EXPIRED, FAILED
		PaymentMethod string `json:"payment_method"`
		PaidAt        string `json:"paid_at"`
	} `json:"transaction"`
}
