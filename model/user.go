package model

import "time"

const (
	PaymentMethodQRIS   = "qris"
	PaymentMethodAZcoin = "azcoin"

	AZTxRedeem   = "redeem"
	AZTxPurchase = "purchase"
	AZTxRefund   = "refund"

	SettingAZcoinDiscountPercent = "azcoin_discount_percent"
	DefaultAZcoinDiscountPercent = 3
)

// User is a storefront customer account (separate from AdminUser).
type User struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	PasswordHash string     `json:"-"`
	Balance      int64      `json:"balance"` // AZcoin balance (1 AZcoin = Rp 1)
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Initial returns the first letter of the user's name (for avatar badges).
func (u *User) Initial() string {
	src := u.Name
	if src == "" {
		src = u.Email
	}
	for _, r := range src {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		return string(r)
	}
	return "?"
}

// AZcoinTransaction is a ledger entry for every balance change.
type AZcoinTransaction struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Type         string    `json:"type"`   // redeem, purchase, refund
	Amount       int64     `json:"amount"` // signed: + credit, - debit
	BalanceAfter int64     `json:"balance_after"`
	Reference    string    `json:"reference"` // voucher code or order ID
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
}

func (t *AZcoinTransaction) TypeLabel() string {
	switch t.Type {
	case AZTxRedeem:
		return "Redeem Voucher"
	case AZTxPurchase:
		return "Pembelian"
	case AZTxRefund:
		return "Refund"
	default:
		return t.Type
	}
}

// Voucher is a single-use AZcoin top-up code sold manually by the admin.
type Voucher struct {
	ID          int64      `json:"id"`
	Code        string     `json:"code"`
	Amount      int64      `json:"amount"`
	Note        string     `json:"note"`
	IsUsed      bool       `json:"is_used"`
	UsedBy      int64      `json:"used_by"`
	UsedByEmail string     `json:"used_by_email,omitempty"`
	UsedAt      *time.Time `json:"used_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AZcoinPrice returns the discounted price when paying with AZcoin.
// The discount never pushes the price below cost (no selling at a loss).
func AZcoinPrice(price, cost, discountPercent int) int {
	if discountPercent <= 0 || price <= 0 {
		return price
	}
	if discountPercent > 90 {
		discountPercent = 90
	}
	az := price - (price*discountPercent)/100
	if cost > 0 && az < cost {
		az = cost
	}
	if az > price {
		az = price
	}
	return az
}
