package model

import "time"

type AdminUser struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	DisplayName  string     `json:"display_name"`
	Role         string     `json:"role"` // superadmin, admin, viewer
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type AdminSession struct {
	Token     string    `json:"token"`
	AdminID   int64     `json:"admin_id"`
	IPAddress string    `json:"ip_address"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type AuditLog struct {
	ID            int64     `json:"id"`
	AdminID       int64     `json:"admin_id"`
	AdminUsername string    `json:"admin_username,omitempty"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	OldValue      string    `json:"old_value"`
	NewValue      string    `json:"new_value"`
	CreatedAt     time.Time `json:"created_at"`
}

type DashboardStats struct {
	TodayOrderCount   int   `json:"today_order_count"`
	TodayRevenue      int64 `json:"today_revenue"`
	TodayProfit       int64 `json:"today_profit"`
	ActiveGameCount   int   `json:"active_game_count"`
	ActiveProductCount int  `json:"active_product_count"`
	PendingOrderCount int   `json:"pending_order_count"`
	DigiflazzBalance  int64 `json:"digiflazz_balance"`
}
