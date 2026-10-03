package model

import "time"

const (
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusProcessing     = "processing"
	StatusSuccess        = "success"
	StatusFailed         = "failed"
	StatusExpired        = "expired"
	StatusRefund         = "refund"
)

type Order struct {
	ID                 string     `json:"id"`
	GameID             int64      `json:"game_id"`
	CustomerNo         string     `json:"customer_no"`
	CustomerNo2        string     `json:"customer_no2"`
	CustomerEmail      string     `json:"customer_email"`
	ProductID          int64      `json:"product_id"`
	ProductName        string     `json:"product_name"`
	Price              int        `json:"price"`
	Cost               int        `json:"cost"`
	Status             string     `json:"status"`
	PaymentTrxID       string     `json:"payment_trx_id"`
	PaymentCheckoutURL string     `json:"payment_checkout_url"`
	PaymentQRURL       string     `json:"payment_qr_url"`
	PaymentQRString    string     `json:"payment_qr_string"`
	PaymentExpiry      *time.Time `json:"payment_expiry"`
	DigiflazzRefID     string     `json:"digiflazz_ref_id"`
	DigiflazzSN        string     `json:"digiflazz_sn"`
	DigiflazzStatus    string     `json:"digiflazz_status"`
	DigiflazzMessage   string     `json:"digiflazz_message"`
	AdminNote          string     `json:"admin_note"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// Joined fields
	GameName string `json:"game_name,omitempty"`
	GameCode string `json:"game_code,omitempty"`
}

func (o *Order) StatusBadge() string {
	switch o.Status {
	case StatusPendingPayment:
		return "Menunggu Pembayaran"
	case StatusPaid:
		return "Dibayar"
	case StatusProcessing:
		return "Sedang Diproses"
	case StatusSuccess:
		return "Sukses"
	case StatusFailed:
		return "Gagal"
	case StatusExpired:
		return "Kadaluarsa"
	case StatusRefund:
		return "Refund"
	default:
		return o.Status
	}
}

func (o *Order) FullCustomerNo() string {
	if o.CustomerNo2 != "" {
		return o.CustomerNo + " (" + o.CustomerNo2 + ")"
	}
	return o.CustomerNo
}
