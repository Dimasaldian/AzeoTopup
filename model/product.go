package model

import "time"

type ProductGroup struct {
	ID        int64     `json:"id"`
	GameID    int64     `json:"game_id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type Product struct {
	ID            int64      `json:"id"`
	GameID        int64      `json:"game_id"`
	DigiflazzSKU  string     `json:"digiflazz_sku"`
	CustomSKU     string     `json:"custom_sku"`
	DisplayName   string     `json:"display_name"`
	DigiflazzName string     `json:"digiflazz_name"`
	Description   string     `json:"description"`
	GroupID       *int64     `json:"group_id"`
	GroupName     string     `json:"group_name,omitempty"`
	CostPrice     int        `json:"cost_price"`
	SellPrice     int        `json:"sell_price"`
	MarginType    string     `json:"margin_type"` // fixed, percent, manual
	MarginValue   int        `json:"margin_value"`
	IconEmoji     string     `json:"icon_emoji"`
	SortOrder     int        `json:"sort_order"`
	IsActive      bool       `json:"is_active"`
	IsPopular     bool       `json:"is_popular"`
	StockStatus   string     `json:"stock_status"` // available, empty, cut_off
	LastSyncAt    *time.Time `json:"last_sync_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Flash Sale / Limited Promo
	IsPromo        bool `json:"is_promo"`
	PromoPrice     int  `json:"promo_price"`
	PromoQuota     int  `json:"promo_quota"`
	PromoRemaining int  `json:"promo_remaining"`

	// Virtual helper fields
	GameName string `json:"game_name,omitempty"`
	GameCode string `json:"game_code,omitempty"`
}

func (p *Product) HasActivePromo() bool {
	return p.IsPromo && p.PromoPrice > 0 && p.PromoRemaining > 0
}

func (p *Product) EffectivePrice() int {
	if p.HasActivePromo() {
		return p.PromoPrice
	}
	return p.SellPrice
}

func (p *Product) OriginalPrice() int {
	return p.SellPrice
}

func (p *Product) SafeGroupID() int64 {
	if p == nil || p.GroupID == nil {
		return 0
	}
	return *p.GroupID
}

func (p *Product) EffectiveSKU() string {
	if p.CustomSKU != "" {
		return p.CustomSKU
	}
	return p.DigiflazzSKU
}

func (p *Product) EffectiveName() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	if p.DigiflazzName != "" {
		return p.DigiflazzName
	}
	return p.DigiflazzSKU
}

// CalculateSellPrice calculates the final customer selling price from cost price,
// margin type ("percent", "fixed", "manual"), and margin value.
//
// For "percent" margin:
// - Calculates margin = costPrice * (marginVal / 100.0)
// - Minimum Floor: If margin is less than Rp 500, it enforces at least Rp 500 profit
//   so micro transactions (e.g. 5 Diamonds) never lose money to payment gateway fees.
// - Rounding (Ceil): Rounds up to the nearest Rp 100 so prices look clean & commercial
//   (e.g., Rp 1.450 -> Rp 2.000, Rp 28.120 -> Rp 28.200).
//
// For "fixed" margin:
// - Adds fixed marginVal directly to costPrice.
// - Rounds up to nearest Rp 100.
func CalculateSellPrice(costPrice int, marginType string, marginVal int) int {
	if costPrice <= 0 {
		return 0
	}

	var rawMargin float64

	switch marginType {
	case "percent":
		rawMargin = float64(costPrice) * (float64(marginVal) / 100.0)
		// Minimum margin floor: Rp 500
		if rawMargin < 500.0 {
			rawMargin = 500.0
		}
	case "fixed":
		rawMargin = float64(marginVal)
	default:
		// manual or fallback
		return costPrice + marginVal
	}

	rawSell := float64(costPrice) + rawMargin

	// Round up (ceil) to nearest 100
	remainder := int(rawSell) % 100
	if remainder > 0 {
		return int(rawSell) + (100 - remainder)
	}

	return int(rawSell)
}

