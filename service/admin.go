package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"topupku/model"
	"topupku/store"
)

type AdminService struct {
	store     *store.SQLiteStore
	digiflazz *DigiflazzClient
}

func NewAdminService(st *store.SQLiteStore, df *DigiflazzClient) *AdminService {
	return &AdminService{
		store:     st,
		digiflazz: df,
	}
}

func (s *AdminService) Authenticate(username, password string) (*model.AdminUser, string, error) {
	admin, err := s.store.GetAdminByUsername(username)
	if err != nil {
		return nil, "", fmt.Errorf("username atau password salah")
	}

	if !admin.IsActive {
		return nil, "", fmt.Errorf("akun dinonaktifkan")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		return nil, "", fmt.Errorf("username atau password salah")
	}

	_ = s.store.UpdateAdminLogin(admin.ID)

	// Generate session token (64 hex characters)
	tokenBytes := make([]byte, 32)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	return admin, token, nil
}

type ImportProductItem struct {
	BuyerSKUCode string
	ProductName  string
	CostPrice    int
	CustomSKU    string
	DisplayName  string
	SellPrice    int
	MarginType   string
	MarginValue  int
	IconEmoji    string
}

func (s *AdminService) ImportProducts(gameID int64, items []ImportProductItem, adminID int64) (int, error) {
	count := 0
	for _, it := range items {
		sellPrice := it.SellPrice
		if sellPrice <= 0 {
			sellPrice = model.CalculateSellPrice(it.CostPrice, it.MarginType, it.MarginValue)
		}

		displayName := it.DisplayName
		if displayName == "" {
			displayName = it.ProductName
		}

		icon := it.IconEmoji
		if icon == "" {
			icon = "💎"
		}

		prod := &model.Product{
			GameID:        gameID,
			DigiflazzSKU:  it.BuyerSKUCode,
			CustomSKU:     it.CustomSKU,
			DisplayName:   displayName,
			DigiflazzName: it.ProductName,
			CostPrice:     it.CostPrice,
			SellPrice:     sellPrice,
			MarginType:    it.MarginType,
			MarginValue:   it.MarginValue,
			IconEmoji:     icon,
			IsActive:      true,
			StockStatus:   "available",
		}

		_, err := s.store.CreateProduct(prod)
		if err == nil {
			count++
		}
	}

	_ = s.store.CreateAuditLog(adminID, "product.import", "game", fmt.Sprintf("%d", gameID), "", fmt.Sprintf("Imported %d products", count))
	return count, nil
}

func (s *AdminService) SyncPriceList(adminID int64) (int, error) {
	// Invalidate in-memory cache to ensure fresh real-time prices from Digiflazz
	s.digiflazz.InvalidateCache()

	items, err := s.digiflazz.GetPriceList("")
	if err != nil {
		return 0, err
	}

	updatedCount := 0
	for _, it := range items {
		stockStatus := "available"
		if !it.BuyerProductStatus || !it.SellerProductStatus {
			stockStatus = "empty"
		}
		if it.Stock == 0 && !it.UnlimitedStock {
			stockStatus = "empty"
		}

		err := s.store.UpsertDigiflazzPriceSync(it.BuyerSKUCode, it.Price, stockStatus)
		if err == nil {
			updatedCount++
		}
	}

	_ = s.store.CreateAuditLog(adminID, "sync.pricelist", "digiflazz", "all", "", fmt.Sprintf("Synced %d items", updatedCount))
	return updatedCount, nil
}

func (s *AdminService) FormatCleanName(rawName, brand string) string {
	clean := strings.TrimSpace(rawName)
	clean = strings.ReplaceAll(clean, strings.ToUpper(brand), "")
	clean = strings.TrimSpace(clean)
	return clean
}
