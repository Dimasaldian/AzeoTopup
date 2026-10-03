package service

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DigiflazzClient struct {
	Username        string
	APIKey          string
	BaseURL         string
	client          *http.Client
	mu              sync.RWMutex
	cachedPriceList []DigiflazzProductItem
	cachedTime      time.Time
}

func NewDigiflazzClient(username, apiKey string) *DigiflazzClient {
	return &DigiflazzClient{
		Username: username,
		APIKey:   apiKey,
		BaseURL:  "https://api.digiflazz.com/v1",
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (d *DigiflazzClient) IsConfigured() bool {
	return d.Username != "" && d.APIKey != ""
}

func (d *DigiflazzClient) Sign(suffix string) string {
	h := md5.Sum([]byte(d.Username + d.APIKey + suffix))
	return hex.EncodeToString(h[:])
}

type DigiflazzBalanceResponse struct {
	Data struct {
		Deposit int64  `json:"deposit"`
		RC      string `json:"rc"`
		Message string `json:"message"`
	} `json:"data"`
}

func (d *DigiflazzClient) CheckBalance() (int64, error) {
	if !d.IsConfigured() {
		// Mocked balance in simulation mode
		return 2500000, nil
	}

	payload := map[string]string{
		"cmd":      "deposit",
		"username": d.Username,
		"sign":     d.Sign("depo"),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	resp, err := d.client.Post(d.BaseURL+"/cek-saldo", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("digiflazz error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result DigiflazzBalanceResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, err
	}

	if result.Data.RC != "" && result.Data.RC != "00" {
		return 0, fmt.Errorf("digiflazz rc %s: %s", result.Data.RC, result.Data.Message)
	}

	return result.Data.Deposit, nil
}

type DigiflazzProductItem struct {
	ProductName         string `json:"product_name"`
	Category            string `json:"category"`
	Brand               string `json:"brand"`
	Type                string `json:"type"`
	SellerName          string `json:"seller_name"`
	Price               int    `json:"price"`
	BuyerSKUCode        string `json:"buyer_sku_code"`
	BuyerProductStatus  bool   `json:"buyer_product_status"`
	SellerProductStatus bool   `json:"seller_product_status"`
	UnlimitedStock      bool   `json:"unlimited_stock"`
	Stock               int    `json:"stock"`
	Multi               bool   `json:"multi"`
	StartCutOff         string `json:"start_cut_off"`
	EndCutOff           string `json:"end_cut_off"`
	Desc                string `json:"desc"`
}

type DigiflazzPriceListResponse struct {
	Data []DigiflazzProductItem `json:"data"`
}

func (d *DigiflazzClient) GetPriceList(brandFilter string) ([]DigiflazzProductItem, error) {
	if !d.IsConfigured() {
		// Return realistic mock items for development / offline use
		return d.getMockPriceList(brandFilter), nil
	}

	// Check if in-memory cache is fresh (valid for 15 minutes)
	d.mu.RLock()
	cachedValid := len(d.cachedPriceList) > 0 && time.Since(d.cachedTime) < 15*time.Minute
	cachedList := d.cachedPriceList
	d.mu.RUnlock()

	if cachedValid {
		return d.filterPriceList(cachedList, brandFilter), nil
	}

	payload := map[string]string{
		"cmd":      "prepaid",
		"username": d.Username,
		"sign":     d.Sign("pricelist"),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Post(d.BaseURL+"/price-list", "application/json", bytes.NewReader(body))
	if err != nil {
		// Fallback to cache if available
		if len(cachedList) > 0 {
			return d.filterPriceList(cachedList, brandFilter), nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("digiflazz error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Digiflazz returns an error object {"data": {"rc": "...", "message": "..."}} on rate limit or failure
	var errResp struct {
		Data struct {
			RC      string `json:"rc"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Data.RC != "" && errResp.Data.RC != "00" {
		// If we hit rate limit (RC: 83) and have older cache, use it!
		if len(cachedList) > 0 {
			return d.filterPriceList(cachedList, brandFilter), nil
		}
		return nil, fmt.Errorf("%s (Kode RC: %s)", errResp.Data.Message, errResp.Data.RC)
	}

	var result DigiflazzPriceListResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		if len(cachedList) > 0 {
			return d.filterPriceList(cachedList, brandFilter), nil
		}
		return nil, fmt.Errorf("format data pricelist tidak valid: %v", err)
	}

	// Save to cache
	d.mu.Lock()
	d.cachedPriceList = result.Data
	d.cachedTime = time.Now()
	d.mu.Unlock()

	return d.filterPriceList(result.Data, brandFilter), nil
}

func (d *DigiflazzClient) filterPriceList(items []DigiflazzProductItem, brandFilter string) []DigiflazzProductItem {
	if brandFilter == "" {
		return items
	}
	var filtered []DigiflazzProductItem
	for _, item := range items {
		if strings.EqualFold(item.Brand, brandFilter) || strings.Contains(strings.ToUpper(item.Brand), strings.ToUpper(brandFilter)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

type DigiflazzTransactionResponse struct {
	Data struct {
		RefID          string `json:"ref_id"`
		CustomerNo     string `json:"customer_no"`
		BuyerSKUCode   string `json:"buyer_sku_code"`
		Message        string `json:"message"`
		Status         string `json:"status"` // Sukses, Pending, Gagal
		RC             string `json:"rc"`     // 00, 03, etc
		SN             string `json:"sn"`
		BuyerLastSaldo int64  `json:"buyer_last_saldo"`
		Price          int    `json:"price"`
	} `json:"data"`
}

func (d *DigiflazzClient) CreateTransaction(buyerSKUCode, customerNo, refID string) (*DigiflazzTransactionResponse, error) {
	if !d.IsConfigured() {
		// Simulation mode
		now := time.Now().Format("20060102150405")
		return &DigiflazzTransactionResponse{
			Data: struct {
				RefID          string `json:"ref_id"`
				CustomerNo     string `json:"customer_no"`
				BuyerSKUCode   string `json:"buyer_sku_code"`
				Message        string `json:"message"`
				Status         string `json:"status"`
				RC             string `json:"rc"`
				SN             string `json:"sn"`
				BuyerLastSaldo int64  `json:"buyer_last_saldo"`
				Price          int    `json:"price"`
			}{
				RefID:          refID,
				CustomerNo:     customerNo,
				BuyerSKUCode:   buyerSKUCode,
				Message:        "Transaksi Berhasil (Simulasi)",
				Status:         "Sukses",
				RC:             "00",
				SN:             fmt.Sprintf("SN-SIM-%s-%s", buyerSKUCode, now),
				BuyerLastSaldo: 2480000,
				Price:          19000,
			},
		}, nil
	}

	payload := map[string]string{
		"username":       d.Username,
		"buyer_sku_code": buyerSKUCode,
		"customer_no":    customerNo,
		"ref_id":         refID,
		"sign":           d.Sign(refID),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Post(d.BaseURL+"/transaction", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result DigiflazzTransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (d *DigiflazzClient) getMockPriceList(brandFilter string) []DigiflazzProductItem {
	allMocks := []DigiflazzProductItem{
		// Mobile Legends
		{ProductName: "MOBILE LEGENDS 86 Diamond", Category: "Games", Brand: "MOBILE LEGENDS", Price: 19000, BuyerSKUCode: "ml-86", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "86 Diamond Mobile Legends"},
		{ProductName: "MOBILE LEGENDS 172 Diamond", Category: "Games", Brand: "MOBILE LEGENDS", Price: 38000, BuyerSKUCode: "ml-172", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "172 Diamond Mobile Legends"},
		{ProductName: "MOBILE LEGENDS 257 Diamond", Category: "Games", Brand: "MOBILE LEGENDS", Price: 57000, BuyerSKUCode: "ml-257", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "257 Diamond Mobile Legends"},
		{ProductName: "MOBILE LEGENDS 344 Diamond", Category: "Games", Brand: "MOBILE LEGENDS", Price: 76000, BuyerSKUCode: "ml-344", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "344 Diamond Mobile Legends"},
		{ProductName: "MOBILE LEGENDS 514 Diamond", Category: "Games", Brand: "MOBILE LEGENDS", Price: 114000, BuyerSKUCode: "ml-514", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "514 Diamond Mobile Legends"},
		{ProductName: "MOBILE LEGENDS 706 Diamond", Category: "Games", Brand: "MOBILE LEGENDS", Price: 152000, BuyerSKUCode: "ml-706", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "706 Diamond Mobile Legends"},
		{ProductName: "MOBILE LEGENDS Weekly Diamond Pass", Category: "Games", Brand: "MOBILE LEGENDS", Price: 27000, BuyerSKUCode: "ml-weekly", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "Weekly Diamond Pass MLBB"},
		{ProductName: "MOBILE LEGENDS Starlight Member", Category: "Games", Brand: "MOBILE LEGENDS", Price: 135000, BuyerSKUCode: "ml-starlight", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "Starlight Member MLBB"},

		// Free Fire
		{ProductName: "FREE FIRE 50 Diamond", Category: "Games", Brand: "FREE FIRE", Price: 6500, BuyerSKUCode: "ff-50", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "50 Diamond Free Fire"},
		{ProductName: "FREE FIRE 70 Diamond", Category: "Games", Brand: "FREE FIRE", Price: 9200, BuyerSKUCode: "ff-70", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "70 Diamond Free Fire"},
		{ProductName: "FREE FIRE 140 Diamond", Category: "Games", Brand: "FREE FIRE", Price: 18500, BuyerSKUCode: "ff-140", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "140 Diamond Free Fire"},
		{ProductName: "FREE FIRE 355 Diamond", Category: "Games", Brand: "FREE FIRE", Price: 46000, BuyerSKUCode: "ff-355", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "355 Diamond Free Fire"},
		{ProductName: "FREE FIRE 720 Diamond", Category: "Games", Brand: "FREE FIRE", Price: 92000, BuyerSKUCode: "ff-720", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "720 Diamond Free Fire"},
		{ProductName: "FREE FIRE Weekly Membership", Category: "Games", Brand: "FREE FIRE", Price: 29000, BuyerSKUCode: "ff-membership", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "Membership Mingguan Free Fire"},

		// PUBG Mobile
		{ProductName: "PUBG MOBILE 60 UC", Category: "Games", Brand: "PUBG MOBILE", Price: 14000, BuyerSKUCode: "pubgm-60", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "60 Unknown Cash PUBG Mobile"},
		{ProductName: "PUBG MOBILE 325 UC", Category: "Games", Brand: "PUBG MOBILE", Price: 70000, BuyerSKUCode: "pubgm-325", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "325 Unknown Cash PUBG Mobile"},
		{ProductName: "PUBG MOBILE 660 UC", Category: "Games", Brand: "PUBG MOBILE", Price: 140000, BuyerSKUCode: "pubgm-660", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "660 Unknown Cash PUBG Mobile"},
		{ProductName: "PUBG MOBILE 1800 UC", Category: "Games", Brand: "PUBG MOBILE", Price: 350000, BuyerSKUCode: "pubgm-1800", BuyerProductStatus: true, SellerProductStatus: true, UnlimitedStock: true, Stock: 999, Desc: "1800 Unknown Cash PUBG Mobile"},
	}

	if brandFilter == "" {
		return allMocks
	}

	var res []DigiflazzProductItem
	for _, it := range allMocks {
		if strings.EqualFold(it.Brand, brandFilter) || strings.Contains(strings.ToUpper(it.Brand), strings.ToUpper(brandFilter)) {
			res = append(res, it)
		}
	}
	return res
}
