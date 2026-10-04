package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"topupku/model"
)

var (
	ErrEmailTaken          = errors.New("email sudah terdaftar")
	ErrInsufficientBalance = errors.New("saldo AZcoin tidak cukup")
	ErrVoucherInvalid      = errors.New("kode voucher tidak valid atau sudah dipakai")
)

func (s *SQLiteStore) migrateUsers() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		balance INTEGER NOT NULL DEFAULT 0 CHECK (balance >= 0),
		is_active INTEGER DEFAULT 1,
		last_login_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS azcoin_transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		amount INTEGER NOT NULL,
		balance_after INTEGER NOT NULL,
		reference TEXT DEFAULT '',
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS vouchers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT UNIQUE NOT NULL,
		amount INTEGER NOT NULL,
		note TEXT DEFAULT '',
		is_used INTEGER DEFAULT 0,
		used_by INTEGER DEFAULT 0,
		used_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_azcoin_tx_user ON azcoin_transactions(user_id);
	CREATE INDEX IF NOT EXISTS idx_azcoin_tx_ref ON azcoin_transactions(reference);
	CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	_, _ = s.db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`,
		model.SettingAZcoinDiscountPercent, strconv.Itoa(model.DefaultAZcoinDiscountPercent))
	return nil
}

// ================= USERS =================

func (s *SQLiteStore) CreateUser(email, name, passwordHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`INSERT INTO users (email, name, password_hash) VALUES (?, ?, ?)`, email, name, passwordHash)
	if err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			return 0, ErrEmailTaken
		}
		return 0, err
	}
	return res.LastInsertId()
}

const userColumns = `id, email, name, password_hash, balance, is_active, last_login_at, created_at`

func scanUser(row interface{ Scan(...interface{}) error }) (*model.User, error) {
	var u model.User
	var active int
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Balance, &active, &u.LastLoginAt, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.IsActive = active == 1
	return &u, nil
}

func (s *SQLiteStore) GetUserByEmail(email string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE email = ?`, email))
}

func (s *SQLiteStore) GetUserByID(id int64) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

func (s *SQLiteStore) UpdateUserLogin(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

type UserListItem struct {
	model.User
	OrderCount int
}

func (s *SQLiteStore) GetAllUsers(search string, limit int) ([]*UserListItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT u.id, u.email, u.name, u.password_hash, u.balance, u.is_active, u.last_login_at, u.created_at,
		       (SELECT COUNT(*) FROM orders o WHERE o.user_id = u.id) AS order_count
		FROM users u`
	var args []interface{}
	if search != "" {
		query += ` WHERE u.email LIKE ? OR u.name LIKE ?`
		like := "%" + search + "%"
		args = append(args, like, like)
	}
	query += ` ORDER BY u.created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*UserListItem
	for rows.Next() {
		var it UserListItem
		var active int
		if err := rows.Scan(&it.ID, &it.Email, &it.Name, &it.PasswordHash, &it.Balance, &active,
			&it.LastLoginAt, &it.CreatedAt, &it.OrderCount); err != nil {
			return nil, err
		}
		it.IsActive = active == 1
		list = append(list, &it)
	}
	return list, nil
}

// ================= USER SESSIONS =================

func (s *SQLiteStore) CreateUserSession(token string, userID int64, maxAgeSec int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	expires := time.Now().Add(time.Duration(maxAgeSec) * time.Second).Unix()
	_, err := s.db.Exec(`INSERT INTO user_sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, expires)
	return err
}

func (s *SQLiteStore) GetUserBySession(token string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return scanUser(s.db.QueryRow(`
		SELECT u.id, u.email, u.name, u.password_hash, u.balance, u.is_active, u.last_login_at, u.created_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ? AND u.is_active = 1
	`, token, time.Now().Unix()))
}

func (s *SQLiteStore) DeleteUserSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM user_sessions WHERE token = ?`, token)
	return err
}

func (s *SQLiteStore) CleanExpiredUserSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM user_sessions WHERE expires_at < ?`, time.Now().Unix())
	return err
}

// ================= SETTINGS =================

func (s *SQLiteStore) GetSetting(key, def string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (s *SQLiteStore) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func (s *SQLiteStore) GetAZcoinDiscountPercent() int {
	v, err := strconv.Atoi(s.GetSetting(model.SettingAZcoinDiscountPercent, strconv.Itoa(model.DefaultAZcoinDiscountPercent)))
	if err != nil || v < 0 {
		return model.DefaultAZcoinDiscountPercent
	}
	return v
}

// ================= VOUCHERS =================

func (s *SQLiteStore) CreateVouchers(codes []string, amount int64, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, c := range codes {
		if _, err := tx.Exec(`INSERT INTO vouchers (code, amount, note) VALUES (?, ?, ?)`, c, amount, note); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetVouchers returns vouchers filtered by status: "unused", "used" or "" (all).
func (s *SQLiteStore) GetVouchers(status string, limit int) ([]*model.Voucher, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT v.id, v.code, v.amount, v.note, v.is_used, v.used_by, COALESCE(u.email, ''), v.used_at, v.created_at
		FROM vouchers v
		LEFT JOIN users u ON u.id = v.used_by`
	switch status {
	case "unused":
		query += ` WHERE v.is_used = 0`
	case "used":
		query += ` WHERE v.is_used = 1`
	}
	query += ` ORDER BY v.created_at DESC, v.id DESC LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*model.Voucher
	for rows.Next() {
		var v model.Voucher
		var used int
		if err := rows.Scan(&v.ID, &v.Code, &v.Amount, &v.Note, &used, &v.UsedBy, &v.UsedByEmail, &v.UsedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.IsUsed = used == 1
		list = append(list, &v)
	}
	return list, nil
}

type VoucherStats struct {
	UnusedCount   int
	UnusedValue   int64
	UsedCount     int
	UsedValue     int64
	TotalBalances int64 // total AZcoin sitting in user wallets (liability)
}

func (s *SQLiteStore) GetVoucherStats() VoucherStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var st VoucherStats
	_ = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(amount), 0) FROM vouchers WHERE is_used = 0`).Scan(&st.UnusedCount, &st.UnusedValue)
	_ = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(amount), 0) FROM vouchers WHERE is_used = 1`).Scan(&st.UsedCount, &st.UsedValue)
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(balance), 0) FROM users`).Scan(&st.TotalBalances)
	return st
}

func (s *SQLiteStore) DeleteUnusedVoucher(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM vouchers WHERE id = ? AND is_used = 0`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("voucher tidak ditemukan atau sudah dipakai")
	}
	return nil
}

// RedeemVoucher atomically marks a voucher as used and credits the user's balance.
func (s *SQLiteStore) RedeemVoucher(userID int64, code string) (amount int64, balanceAfter int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	var voucherID int64
	err = tx.QueryRow(`SELECT id, amount FROM vouchers WHERE code = ? AND is_used = 0`, code).Scan(&voucherID, &amount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, ErrVoucherInvalid
		}
		return 0, 0, err
	}

	res, err := tx.Exec(`UPDATE vouchers SET is_used = 1, used_by = ?, used_at = CURRENT_TIMESTAMP WHERE id = ? AND is_used = 0`, userID, voucherID)
	if err != nil {
		return 0, 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, 0, ErrVoucherInvalid
	}

	if _, err = tx.Exec(`UPDATE users SET balance = balance + ? WHERE id = ?`, amount, userID); err != nil {
		return 0, 0, err
	}
	if err = tx.QueryRow(`SELECT balance FROM users WHERE id = ?`, userID).Scan(&balanceAfter); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(`
		INSERT INTO azcoin_transactions (user_id, type, amount, balance_after, reference, description)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, model.AZTxRedeem, amount, balanceAfter, code, "Redeem voucher AZcoin"); err != nil {
		return 0, 0, err
	}

	return amount, balanceAfter, tx.Commit()
}

// ================= AZCOIN ORDERS =================

// CreateAZcoinOrder deducts the user's balance and inserts the (already paid) order in one transaction.
func (s *SQLiteStore) CreateAZcoinOrder(o *model.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if o.UserID <= 0 {
		return fmt.Errorf("user wajib login untuk bayar dengan AZcoin")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE users SET balance = balance - ? WHERE id = ? AND balance >= ? AND is_active = 1`, o.Price, o.UserID, o.Price)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInsufficientBalance
	}

	var balanceAfter int64
	if err := tx.QueryRow(`SELECT balance FROM users WHERE id = ?`, o.UserID).Scan(&balanceAfter); err != nil {
		return err
	}

	o.PaymentMethod = model.PaymentMethodAZcoin
	if err := insertOrder(tx, o); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO azcoin_transactions (user_id, type, amount, balance_after, reference, description)
		VALUES (?, ?, ?, ?, ?, ?)
	`, o.UserID, model.AZTxPurchase, -int64(o.Price), balanceAfter, o.ID, "Pembelian "+o.ProductName); err != nil {
		return err
	}

	return tx.Commit()
}

// RefundAZcoinOrder returns the AZcoin spent on an order back to the user's wallet.
// It is idempotent: an order is refunded at most once. Returns true if a refund happened.
func (s *SQLiteStore) RefundAZcoinOrder(orderID, reason string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var userID int64
	var method string
	var price int64
	err = tx.QueryRow(`SELECT COALESCE(user_id, 0), COALESCE(payment_method, 'qris'), price FROM orders WHERE id = ?`, orderID).
		Scan(&userID, &method, &price)
	if err != nil {
		return false, err
	}
	if method != model.PaymentMethodAZcoin || userID <= 0 {
		return false, nil
	}

	var already int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM azcoin_transactions WHERE type = ? AND reference = ?`, model.AZTxRefund, orderID).Scan(&already)
	if already > 0 {
		return false, nil
	}

	if _, err := tx.Exec(`UPDATE users SET balance = balance + ? WHERE id = ?`, price, userID); err != nil {
		return false, err
	}
	var balanceAfter int64
	if err := tx.QueryRow(`SELECT balance FROM users WHERE id = ?`, userID).Scan(&balanceAfter); err != nil {
		return false, err
	}
	if reason == "" {
		reason = "Refund otomatis pesanan gagal"
	}
	if _, err := tx.Exec(`
		INSERT INTO azcoin_transactions (user_id, type, amount, balance_after, reference, description)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, model.AZTxRefund, price, balanceAfter, orderID, reason); err != nil {
		return false, err
	}

	return true, tx.Commit()
}

func (s *SQLiteStore) HasAZcoinRefund(orderID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM azcoin_transactions WHERE type = ? AND reference = ?`, model.AZTxRefund, orderID).Scan(&n)
	return n > 0
}

func (s *SQLiteStore) GetAZcoinTransactions(userID int64, limit int) ([]*model.AZcoinTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, user_id, type, amount, balance_after, reference, description, created_at
		FROM azcoin_transactions WHERE user_id = ?
		ORDER BY id DESC LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*model.AZcoinTransaction
	for rows.Next() {
		var t model.AZcoinTransaction
		if err := rows.Scan(&t.ID, &t.UserID, &t.Type, &t.Amount, &t.BalanceAfter, &t.Reference, &t.Description, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &t)
	}
	return list, nil
}

func (s *SQLiteStore) GetOrdersByUserID(userID int64, limit int) ([]*model.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT o.id, o.game_id, o.customer_no, o.customer_no2, o.customer_email, o.product_id, o.product_name,
		       o.price, o.status, o.digiflazz_sn, o.created_at,
		       g.name, g.code, COALESCE(o.payment_method, 'qris')
		FROM orders o
		JOIN games g ON o.game_id = g.id
		WHERE o.user_id = ?
		ORDER BY o.created_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*model.Order
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.ID, &o.GameID, &o.CustomerNo, &o.CustomerNo2, &o.CustomerEmail, &o.ProductID, &o.ProductName,
			&o.Price, &o.Status, &o.DigiflazzSN, &o.CreatedAt, &o.GameName, &o.GameCode, &o.PaymentMethod); err != nil {
			return nil, err
		}
		o.UserID = userID
		list = append(list, &o)
	}
	return list, nil
}
