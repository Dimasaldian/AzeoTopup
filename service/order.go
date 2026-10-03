package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"topupku/model"
	"topupku/store"
)

type OrderService struct {
	store     *store.SQLiteStore
	autogopay *AutoGoPayClient
	digiflazz *DigiflazzClient
	email     *EmailService
}

func NewOrderService(st *store.SQLiteStore, agp *AutoGoPayClient, df *DigiflazzClient, email *EmailService) *OrderService {
	return &OrderService{
		store:     st,
		autogopay: agp,
		digiflazz: df,
		email:     email,
	}
}

func (s *OrderService) GenerateOrderID() string {
	datePart := time.Now().Format("20060102")
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	randPart := make([]byte, 4)
	for i := range randPart {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		randPart[i] = letters[num.Int64()]
	}
	return fmt.Sprintf("TK-%s-%s", datePart, string(randPart))
}

func (s *OrderService) CreateOrder(gameCode string, customerNo, customerNo2, customerEmail string, productID int64) (*model.Order, error) {
	game, err := s.store.GetGameByCode(gameCode)
	if err != nil {
		return nil, fmt.Errorf("game tidak ditemukan")
	}
	if !game.IsActive {
		return nil, fmt.Errorf("game sedang tidak aktif")
	}

	product, err := s.store.GetProductByID(productID)
	if err != nil {
		return nil, fmt.Errorf("produk tidak ditemukan")
	}
	if !product.IsActive {
		return nil, fmt.Errorf("produk sedang tidak aktif")
	}
	if product.GameID != game.ID {
		return nil, fmt.Errorf("produk tidak sesuai dengan game yang dipilih")
	}

	customerNo = strings.TrimSpace(customerNo)
	customerNo2 = strings.TrimSpace(customerNo2)
	customerEmail = strings.TrimSpace(customerEmail)
	if customerNo == "" {
		return nil, fmt.Errorf("nomor ID akun wajib diisi")
	}
	if customerEmail == "" {
		return nil, fmt.Errorf("alamat email wajib diisi untuk pengiriman invoice")
	}
	if !strings.Contains(customerEmail, "@") || !strings.Contains(customerEmail, ".") {
		return nil, fmt.Errorf("format alamat email tidak valid")
	}

	orderID := s.GenerateOrderID()

	// Use EffectivePrice (takes PromoPrice if flash sale promo is active)
	finalPrice := product.EffectivePrice()

	// Generate QRIS via AutoGoPay
	qrisData, err := s.autogopay.GenerateQRIS(orderID, finalPrice)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat pembayaran QRIS: %w", err)
	}

	var expiryTime *time.Time
	if qrisData.ExpiryTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", qrisData.ExpiryTime); err == nil {
			expiryTime = &t
		}
	}
	if expiryTime == nil {
		t := time.Now().Add(15 * time.Minute)
		expiryTime = &t
	}

	order := &model.Order{
		ID:                 orderID,
		GameID:             game.ID,
		CustomerNo:         customerNo,
		CustomerNo2:        customerNo2,
		CustomerEmail:      customerEmail,
		ProductID:          product.ID,
		ProductName:        product.EffectiveName(),
		Price:              finalPrice,
		Cost:               product.CostPrice,
		Status:             model.StatusPendingPayment,
		PaymentTrxID:       qrisData.TransactionID,
		PaymentCheckoutURL: qrisData.CheckoutURL,
		PaymentQRURL:       qrisData.QRURL,
		PaymentQRString:    qrisData.QRString,
		PaymentExpiry:      expiryTime,
		DigiflazzRefID:     orderID,
	}

	if err := s.store.CreateOrder(order); err != nil {
		return nil, fmt.Errorf("gagal menyimpan pesanan: %w", err)
	}

	order.GameName = game.Name
	order.GameCode = game.Code
	return order, nil
}

func (s *OrderService) ProcessPaymentReceived(paymentTrxID string) error {
	order, err := s.store.GetOrderByPaymentTrxID(paymentTrxID)
	if err != nil {
		return fmt.Errorf("order dengan trx ID %s tidak ditemukan: %w", paymentTrxID, err)
	}

	// Idempotency: if already paid or further along, skip
	if order.Status == model.StatusPaid || order.Status == model.StatusProcessing || order.Status == model.StatusSuccess {
		return nil
	}

	// Update to paid
	_ = s.store.UpdateOrderStatus(order.ID, model.StatusPaid)

	// Deduct promo quota if product had promo
	_ = s.store.DeductProductPromoQuota(order.ProductID)

	// Trigger top up via Digiflazz
	return s.ExecuteTopUp(order)
}

func (s *OrderService) ExecuteTopUp(order *model.Order) error {
	product, err := s.store.GetProductByID(order.ProductID)
	if err != nil {
		_ = s.store.UpdateOrderStatus(order.ID, model.StatusProcessing)
		return err
	}

	// Format destination ID for Digiflazz
	// Mobile Legends: User ID + Zone ID (e.g. 123456782134)
	destCustomerNo := order.CustomerNo
	if order.CustomerNo2 != "" {
		destCustomerNo = order.CustomerNo + order.CustomerNo2
	}

	_ = s.store.UpdateOrderStatus(order.ID, model.StatusProcessing)

	dfResp, err := s.digiflazz.CreateTransaction(product.DigiflazzSKU, destCustomerNo, order.ID)
	if err != nil {
		_ = s.store.UpdateOrderDigiflazz(order.ID, order.ID, "Gagal", "", err.Error())
		_ = s.store.UpdateOrderStatus(order.ID, model.StatusFailed)
		return err
	}

	statusLower := strings.ToLower(dfResp.Data.Status)
	var finalStatus string

	if statusLower == "sukses" || statusLower == "success" {
		finalStatus = model.StatusSuccess
	} else if statusLower == "gagal" || statusLower == "failed" {
		finalStatus = model.StatusFailed
	} else {
		finalStatus = model.StatusProcessing
	}

	_ = s.store.UpdateOrderDigiflazz(order.ID, dfResp.Data.RefID, dfResp.Data.Status, dfResp.Data.SN, dfResp.Data.Message)
	_ = s.store.UpdateOrderStatus(order.ID, finalStatus)
	order.Status = finalStatus
	order.DigiflazzSN = dfResp.Data.SN
	order.DigiflazzMessage = dfResp.Data.Message

	if finalStatus == model.StatusSuccess && s.email != nil && order.CustomerEmail != "" {
		go func(o model.Order) {
			_ = s.email.SendOrderReceipt(&o)
		}(*order)
	}

	return nil
}

func (s *OrderService) ProcessDigiflazzWebhook(refID, status, sn, message string) error {
	order, err := s.store.GetOrderByDigiflazzRefID(refID)
	if err != nil {
		order, err = s.store.GetOrderByID(refID)
		if err != nil {
			return fmt.Errorf("order dengan ref ID %s tidak ditemukan: %w", refID, err)
		}
	}

	statusLower := strings.ToLower(status)
	var finalStatus string
	if statusLower == "sukses" || statusLower == "success" {
		finalStatus = model.StatusSuccess
	} else if statusLower == "gagal" || statusLower == "failed" {
		finalStatus = model.StatusFailed
	} else {
		finalStatus = model.StatusProcessing
	}

	if err := s.store.UpdateOrderDigiflazz(order.ID, refID, status, sn, message); err != nil {
		return err
	}
	_ = s.store.UpdateOrderStatus(order.ID, finalStatus)
	order.Status = finalStatus
	order.DigiflazzSN = sn
	order.DigiflazzMessage = message

	if finalStatus == model.StatusSuccess && s.email != nil && order.CustomerEmail != "" {
		go func(o model.Order) {
			_ = s.email.SendOrderReceipt(&o)
		}(*order)
	}

	return nil
}

func (s *OrderService) CheckPendingOrderDigiflazzStatus(order *model.Order) (*model.Order, error) {
	if order.Status != model.StatusProcessing {
		return order, nil
	}

	product, err := s.store.GetProductByID(order.ProductID)
	if err != nil {
		return order, err
	}

	destCustomerNo := order.CustomerNo
	if order.CustomerNo2 != "" {
		destCustomerNo = order.CustomerNo + order.CustomerNo2
	}

	// Digiflazz returns existing transaction status when called with identical SKU, CustomerNo, and RefID
	dfResp, err := s.digiflazz.CreateTransaction(product.DigiflazzSKU, destCustomerNo, order.ID)
	if err != nil {
		return order, err
	}

	statusLower := strings.ToLower(dfResp.Data.Status)
	if statusLower == "sukses" || statusLower == "success" {
		_ = s.store.UpdateOrderDigiflazz(order.ID, dfResp.Data.RefID, dfResp.Data.Status, dfResp.Data.SN, dfResp.Data.Message)
		_ = s.store.UpdateOrderStatus(order.ID, model.StatusSuccess)
		order.Status = model.StatusSuccess
		order.DigiflazzSN = dfResp.Data.SN
		order.DigiflazzMessage = dfResp.Data.Message

		if s.email != nil && order.CustomerEmail != "" {
			go func(o model.Order) {
				_ = s.email.SendOrderReceipt(&o)
			}(*order)
		}
	} else if statusLower == "gagal" || statusLower == "failed" {
		_ = s.store.UpdateOrderDigiflazz(order.ID, dfResp.Data.RefID, dfResp.Data.Status, dfResp.Data.SN, dfResp.Data.Message)
		_ = s.store.UpdateOrderStatus(order.ID, model.StatusFailed)
		order.Status = model.StatusFailed
		order.DigiflazzMessage = dfResp.Data.Message
	}

	return order, nil
}

func (s *OrderService) SyncProcessingOrders() (int, error) {
	orders, err := s.store.GetProcessingOrders()
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, o := range orders {
		updatedOrder, err := s.CheckPendingOrderDigiflazzStatus(o)
		if err == nil && updatedOrder.Status != model.StatusProcessing {
			updated++
		}
	}
	return updated, nil
}

func (s *OrderService) GetOrderByIDWithLiveCheck(orderID string) (*model.Order, error) {
	order, err := s.store.GetOrderByID(orderID)
	if err != nil {
		return nil, err
	}

	if order.Status == model.StatusProcessing {
		order, _ = s.CheckPendingOrderDigiflazzStatus(order)
	}

	return order, nil
}

func (s *OrderService) RetryTopUp(orderID string) error {
	order, err := s.store.GetOrderByID(orderID)
	if err != nil {
		return err
	}

	return s.ExecuteTopUp(order)
}

func (s *OrderService) RefundOrder(orderID, adminNote string) error {
	return s.store.UpdateOrderNote(orderID, adminNote, model.StatusRefund)
}

func (s *OrderService) CheckExpiredOrders() (int64, error) {
	return s.store.ExpireOldPendingOrders()
}
