package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"topupku/model"
)

type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// SQLite connection string with WAL mode and foreign keys enabled
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite works safest with single writer
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}
	_ = store.AutoAssignProductGroups()

	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS games (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		brand TEXT NOT NULL,
		description TEXT DEFAULT '',
		icon_path TEXT DEFAULT '',
		banner_path TEXT DEFAULT '',
		id_label TEXT NOT NULL DEFAULT 'User ID',
		id_placeholder TEXT NOT NULL DEFAULT 'Masukkan User ID',
		id_help_text TEXT DEFAULT '',
		id2_label TEXT DEFAULT '',
		id2_placeholder TEXT DEFAULT '',
		id_format_regex TEXT DEFAULT '',
		instruction_text TEXT DEFAULT '',
		sort_order INTEGER DEFAULT 0,
		is_active INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS product_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		sort_order INTEGER DEFAULT 0,
		is_active INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
		digiflazz_sku TEXT NOT NULL,
		custom_sku TEXT UNIQUE,
		display_name TEXT NOT NULL,
		digiflazz_name TEXT NOT NULL,
		description TEXT DEFAULT '',
		group_id INTEGER REFERENCES product_groups(id) ON DELETE SET NULL,
		cost_price INTEGER NOT NULL DEFAULT 0,
		sell_price INTEGER NOT NULL DEFAULT 0,
		margin_type TEXT DEFAULT 'fixed',
		margin_value INTEGER DEFAULT 0,
		icon_emoji TEXT DEFAULT '💎',
		sort_order INTEGER DEFAULT 0,
		is_active INTEGER DEFAULT 1,
		is_popular INTEGER DEFAULT 0,
		stock_status TEXT DEFAULT 'available',
		last_sync_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS orders (
		id TEXT PRIMARY KEY,
		game_id INTEGER NOT NULL REFERENCES games(id),
		customer_no TEXT NOT NULL,
		customer_no2 TEXT DEFAULT '',
		customer_email TEXT DEFAULT '',
		product_id INTEGER NOT NULL REFERENCES products(id),
		product_name TEXT NOT NULL,
		price INTEGER NOT NULL,
		cost INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending_payment',
		payment_trx_id TEXT DEFAULT '',
		payment_checkout_url TEXT DEFAULT '',
		payment_qr_url TEXT DEFAULT '',
		payment_qr_string TEXT DEFAULT '',
		payment_expiry DATETIME,
		digiflazz_ref_id TEXT DEFAULT '',
		digiflazz_sn TEXT DEFAULT '',
		digiflazz_status TEXT DEFAULT '',
		digiflazz_message TEXT DEFAULT '',
		admin_note TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS admin_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		display_name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'admin',
		is_active INTEGER DEFAULT 1,
		last_login_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS admin_sessions (
		token TEXT PRIMARY KEY,
		admin_id INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
		ip_address TEXT DEFAULT '',
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		admin_id INTEGER,
		action TEXT NOT NULL,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		old_value TEXT DEFAULT '',
		new_value TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
	CREATE INDEX IF NOT EXISTS idx_orders_created ON orders(created_at);
	CREATE INDEX IF NOT EXISTS idx_products_game ON products(game_id);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Auto-migrate newly added columns
	_, _ = s.db.Exec("ALTER TABLE orders ADD COLUMN customer_email TEXT DEFAULT ''")
	_, _ = s.db.Exec("ALTER TABLE products ADD COLUMN is_promo INTEGER DEFAULT 0")
	_, _ = s.db.Exec("ALTER TABLE products ADD COLUMN promo_price INTEGER DEFAULT 0")
	_, _ = s.db.Exec("ALTER TABLE products ADD COLUMN promo_quota INTEGER DEFAULT 0")
	_, _ = s.db.Exec("ALTER TABLE products ADD COLUMN promo_remaining INTEGER DEFAULT 0")
	_, _ = s.db.Exec("ALTER TABLE orders ADD COLUMN user_id INTEGER DEFAULT 0")
	_, _ = s.db.Exec("ALTER TABLE orders ADD COLUMN payment_method TEXT DEFAULT 'qris'")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id)")
	_, _ = s.db.Exec("PRAGMA foreign_keys = OFF")
	_, _ = s.db.Exec("DELETE FROM products WHERE id = 1 AND is_active = 0")
	_, _ = s.db.Exec("PRAGMA foreign_keys = ON")

	if err := s.migrateUsers(); err != nil {
		return err
	}
	return s.migrateTickets()
}

func (s *SQLiteStore) SeedInitialData(adminUsername, adminPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Seed Admin if not present
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count)
	if err == nil && count == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err == nil {
			_, _ = s.db.Exec(`
				INSERT INTO admin_users (username, password_hash, display_name, role, is_active)
				VALUES (?, ?, ?, 'superadmin', 1)
			`, adminUsername, string(hash), "Super Admin")
		}
	}

	// Seed Games if not present
	err = s.db.QueryRow("SELECT COUNT(*) FROM games").Scan(&count)
	if err == nil && count == 0 {
		games := []struct {
			code, name, brand, desc, icon, idLabel, idPlaceholder, idHelp, id2Label, id2Placeholder, idRegex, inst string
		}{
			{
				code: "ml", name: "Mobile Legends", brand: "MOBILE LEGENDS",
				desc: "Top up Diamond Mobile Legends: Bang Bang instan 24 jam",
				icon: "https://images.unsplash.com/photo-1542751371-adc38448a05e?w=128&q=80",
				idLabel: "User ID", idPlaceholder: "Contoh: 12345678",
				idHelp: "Buka menu profil di dalam game untuk melihat User ID Anda",
				id2Label: "Zone ID", id2Placeholder: "Contoh: 2134",
				idRegex: `^\d{5,12}$`,
				inst: "1. Masukkan User ID dan Zone ID Anda.<br/>2. Pilih nominal Diamond yang diinginkan.<br/>3. Selesaikan pembayaran dengan QRIS.<br/>4. Diamond langsung masuk ke akun MLBB Anda!",
			},
			{
				code: "ff", name: "Free Fire", brand: "FREE FIRE",
				desc: "Top up Diamond Garena Free Fire cepat dan resmi",
				icon: "https://images.unsplash.com/photo-1538481199705-c710c4e965fc?w=128&q=80",
				idLabel: "Player ID", idPlaceholder: "Contoh: 876543210",
				idHelp: "Buka profil Free Fire Anda, ID berupa 8-10 digit angka",
				id2Label: "", id2Placeholder: "",
				idRegex: `^\d{7,12}$`,
				inst: "1. Masukkan Player ID Free Fire Anda.<br/>2. Pilih jumlah Diamond yang diinginkan.<br/>3. Scan QRIS untuk membayar.<br/>4. Diamond instan terisi dalam hitungan detik!",
			},
			{
				code: "pubgm", name: "PUBG Mobile", brand: "PUBG MOBILE",
				desc: "Isi Unknown Cash (UC) PUBG Mobile murah dan terpercaya",
				icon: "https://images.unsplash.com/photo-1550745165-9bc0b252726f?w=128&q=80",
				idLabel: "Player ID (UID)", idPlaceholder: "Contoh: 5123456789",
				idHelp: "Ketuk avatar profil Anda di pojok kiri atas untuk melihat UID",
				id2Label: "", id2Placeholder: "",
				idRegex: `^\d{8,12}$`,
				inst: "1. Masukkan ID Akun PUBG Mobile Anda.<br/>2. Pilih nominal Unknown Cash (UC).<br/>3. Bayar menggunakan QRIS e-wallet/m-banking.<br/>4. UC langsung ditambahkan ke inventori game!",
			},
		}

		for idx, g := range games {
			res, err := s.db.Exec(`
				INSERT INTO games (code, name, brand, description, icon_path, id_label, id_placeholder, id_help_text, id2_label, id2_placeholder, id_format_regex, instruction_text, sort_order, is_active)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
			`, g.code, g.name, g.brand, g.desc, g.icon, g.idLabel, g.idPlaceholder, g.idHelp, g.id2Label, g.id2Placeholder, g.idRegex, g.inst, idx)
			if err != nil {
				continue
			}
			gameID, _ := res.LastInsertId()

			// Seed sample products for each game
			if g.code == "ml" {
				// ML products
				mlProducts := []struct {
					sku, customSku, name string
					cost, sell           int
					pop                  int
				}{
					{"ml-86", "TK-ML-86", "86 Diamond", 19000, 20500, 1},
					{"ml-172", "TK-ML-172", "172 Diamond", 38000, 41000, 1},
					{"ml-257", "TK-ML-257", "257 Diamond", 57000, 61500, 0},
					{"ml-344", "TK-ML-344", "344 Diamond", 76000, 82000, 0},
					{"ml-514", "TK-ML-514", "514 Diamond", 114000, 122000, 0},
					{"ml-706", "TK-ML-706", "706 Diamond", 152000, 163000, 1},
					{"ml-weekly", "TK-ML-PASS", "Weekly Diamond Pass", 27000, 29500, 1},
				}
				for pIdx, p := range mlProducts {
					_, _ = s.db.Exec(`
						INSERT INTO products (game_id, digiflazz_sku, custom_sku, display_name, digiflazz_name, cost_price, sell_price, margin_type, margin_value, icon_emoji, sort_order, is_active, is_popular, stock_status)
						VALUES (?, ?, ?, ?, ?, ?, ?, 'fixed', ?, '💎', ?, 1, ?, 'available')
					`, gameID, p.sku, p.customSku, p.name, "MOBILE LEGENDS "+p.name, p.cost, p.sell, p.sell-p.cost, pIdx, p.pop)
				}
			} else if g.code == "ff" {
				ffProducts := []struct {
					sku, customSku, name string
					cost, sell           int
					pop                  int
				}{
					{"ff-50", "TK-FF-50", "50 Diamond", 6500, 7500, 0},
					{"ff-70", "TK-FF-70", "70 Diamond", 9200, 10500, 1},
					{"ff-140", "TK-FF-140", "140 Diamond", 18500, 20500, 1},
					{"ff-355", "TK-FF-355", "355 Diamond", 46000, 50000, 1},
					{"ff-720", "TK-FF-720", "720 Diamond", 92000, 99000, 0},
					{"ff-membership", "TK-FF-WEEKLY", "Weekly Membership", 29000, 32000, 1},
				}
				for pIdx, p := range ffProducts {
					_, _ = s.db.Exec(`
						INSERT INTO products (game_id, digiflazz_sku, custom_sku, display_name, digiflazz_name, cost_price, sell_price, margin_type, margin_value, icon_emoji, sort_order, is_active, is_popular, stock_status)
						VALUES (?, ?, ?, ?, ?, ?, ?, 'fixed', ?, '💎', ?, 1, ?, 'available')
					`, gameID, p.sku, p.customSku, p.name, "FREE FIRE "+p.name, p.cost, p.sell, p.sell-p.cost, pIdx, p.pop)
				}
			} else if g.code == "pubgm" {
				pubgProducts := []struct {
					sku, customSku, name string
					cost, sell           int
					pop                  int
				}{
					{"pubgm-60", "TK-PUBG-60", "60 UC", 14000, 15500, 0},
					{"pubgm-325", "TK-PUBG-325", "325 UC", 70000, 76000, 1},
					{"pubgm-660", "TK-PUBG-660", "660 UC", 140000, 151000, 1},
					{"pubgm-1800", "TK-PUBG-1800", "1800 UC", 350000, 375000, 0},
				}
				for pIdx, p := range pubgProducts {
					_, _ = s.db.Exec(`
						INSERT INTO products (game_id, digiflazz_sku, custom_sku, display_name, digiflazz_name, cost_price, sell_price, margin_type, margin_value, icon_emoji, sort_order, is_active, is_popular, stock_status)
						VALUES (?, ?, ?, ?, ?, ?, ?, 'fixed', ?, '🪙', ?, 1, ?, 'available')
					`, gameID, p.sku, p.customSku, p.name, "PUBG MOBILE "+p.name, p.cost, p.sell, p.sell-p.cost, pIdx, p.pop)
				}
			}
		}
	}

	return nil
}

// ================= GAME STORE =================

func (s *SQLiteStore) GetAllGames(onlyActive bool) ([]*model.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT g.id, g.code, g.name, g.brand, g.description, g.icon_path, g.banner_path,
		       g.id_label, g.id_placeholder, g.id_help_text, g.id2_label, g.id2_placeholder,
		       g.id_format_regex, g.instruction_text, g.sort_order, g.is_active,
		       g.created_at, g.updated_at,
		       (SELECT COUNT(*) FROM products p WHERE p.game_id = g.id AND p.is_active = 1) as product_count
		FROM games g
	`
	if onlyActive {
		query += " WHERE g.is_active = 1"
	}
	query += " ORDER BY g.sort_order ASC, g.id ASC"

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*model.Game
	for rows.Next() {
		var g model.Game
		var isActiveInt int
		err := rows.Scan(
			&g.ID, &g.Code, &g.Name, &g.Brand, &g.Description, &g.IconPath, &g.BannerPath,
			&g.IDLabel, &g.IDPlaceholder, &g.IDHelpText, &g.ID2Label, &g.ID2Placeholder,
			&g.IDFormatRegex, &g.InstructionText, &g.SortOrder, &isActiveInt,
			&g.CreatedAt, &g.UpdatedAt, &g.ProductCount,
		)
		if err != nil {
			return nil, err
		}
		g.IsActive = isActiveInt == 1
		games = append(games, &g)
	}

	return games, nil
}

func (s *SQLiteStore) GetGameByID(id int64) (*model.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT g.id, g.code, g.name, g.brand, g.description, g.icon_path, g.banner_path,
		       g.id_label, g.id_placeholder, g.id_help_text, g.id2_label, g.id2_placeholder,
		       g.id_format_regex, g.instruction_text, g.sort_order, g.is_active,
		       g.created_at, g.updated_at,
		       (SELECT COUNT(*) FROM products p WHERE p.game_id = g.id) as product_count
		FROM games g WHERE g.id = ?
	`
	var g model.Game
	var isActiveInt int
	err := s.db.QueryRow(query, id).Scan(
		&g.ID, &g.Code, &g.Name, &g.Brand, &g.Description, &g.IconPath, &g.BannerPath,
		&g.IDLabel, &g.IDPlaceholder, &g.IDHelpText, &g.ID2Label, &g.ID2Placeholder,
		&g.IDFormatRegex, &g.InstructionText, &g.SortOrder, &isActiveInt,
		&g.CreatedAt, &g.UpdatedAt, &g.ProductCount,
	)
	if err != nil {
		return nil, err
	}
	g.IsActive = isActiveInt == 1
	return &g, nil
}

func (s *SQLiteStore) GetGameByCode(code string) (*model.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT g.id, g.code, g.name, g.brand, g.description, g.icon_path, g.banner_path,
		       g.id_label, g.id_placeholder, g.id_help_text, g.id2_label, g.id2_placeholder,
		       g.id_format_regex, g.instruction_text, g.sort_order, g.is_active,
		       g.created_at, g.updated_at,
		       (SELECT COUNT(*) FROM products p WHERE p.game_id = g.id AND p.is_active = 1) as product_count
		FROM games g WHERE LOWER(g.code) = LOWER(?)
	`
	var g model.Game
	var isActiveInt int
	err := s.db.QueryRow(query, code).Scan(
		&g.ID, &g.Code, &g.Name, &g.Brand, &g.Description, &g.IconPath, &g.BannerPath,
		&g.IDLabel, &g.IDPlaceholder, &g.IDHelpText, &g.ID2Label, &g.ID2Placeholder,
		&g.IDFormatRegex, &g.InstructionText, &g.SortOrder, &isActiveInt,
		&g.CreatedAt, &g.UpdatedAt, &g.ProductCount,
	)
	if err != nil {
		return nil, err
	}
	g.IsActive = isActiveInt == 1
	return &g, nil
}

func (s *SQLiteStore) CreateGame(g *model.Game) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`
		INSERT INTO games (code, name, brand, description, icon_path, banner_path, id_label, id_placeholder, id_help_text, id2_label, id2_placeholder, id_format_regex, instruction_text, sort_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, g.Code, g.Name, g.Brand, g.Description, g.IconPath, g.BannerPath, g.IDLabel, g.IDPlaceholder, g.IDHelpText, g.ID2Label, g.ID2Placeholder, g.IDFormatRegex, g.InstructionText, g.SortOrder, boolToInt(g.IsActive))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) UpdateGame(g *model.Game) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE games SET
			code = ?, name = ?, brand = ?, description = ?,
			icon_path = CASE WHEN ? != '' THEN ? ELSE icon_path END,
			banner_path = CASE WHEN ? != '' THEN ? ELSE banner_path END,
			id_label = ?, id_placeholder = ?, id_help_text = ?,
			id2_label = ?, id2_placeholder = ?, id_format_regex = ?,
			instruction_text = ?, sort_order = ?, is_active = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, g.Code, g.Name, g.Brand, g.Description, g.IconPath, g.IconPath, g.BannerPath, g.BannerPath, g.IDLabel, g.IDPlaceholder, g.IDHelpText, g.ID2Label, g.ID2Placeholder, g.IDFormatRegex, g.InstructionText, g.SortOrder, boolToInt(g.IsActive), g.ID)
	return err
}

func (s *SQLiteStore) ToggleGame(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`UPDATE games SET is_active = 1 - is_active, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) DeleteGame(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = s.db.Exec("PRAGMA foreign_keys = OFF")
	defer func() {
		_, _ = s.db.Exec("PRAGMA foreign_keys = ON")
	}()

	_, err := s.db.Exec("DELETE FROM games WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) ReorderGame(id int64, direction string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var currentSort int
	err := s.db.QueryRow("SELECT sort_order FROM games WHERE id = ?", id).Scan(&currentSort)
	if err != nil {
		return err
	}

	var neighborID int64
	var neighborSort int
	if direction == "up" {
		err = s.db.QueryRow("SELECT id, sort_order FROM games WHERE sort_order < ? ORDER BY sort_order DESC LIMIT 1", currentSort).Scan(&neighborID, &neighborSort)
	} else {
		err = s.db.QueryRow("SELECT id, sort_order FROM games WHERE sort_order > ? ORDER BY sort_order ASC LIMIT 1", currentSort).Scan(&neighborID, &neighborSort)
	}
	if err != nil {
		return nil // already top or bottom
	}

	_, _ = s.db.Exec("UPDATE games SET sort_order = ? WHERE id = ?", neighborSort, id)
	_, _ = s.db.Exec("UPDATE games SET sort_order = ? WHERE id = ?", currentSort, neighborID)
	return nil
}

// ================= PRODUCT STORE =================

func (s *SQLiteStore) GetProductsByGameID(gameID int64, onlyActive bool) ([]*model.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT p.id, p.game_id, p.digiflazz_sku, p.custom_sku, p.display_name, p.digiflazz_name,
		       p.description, p.group_id, COALESCE(pg.name, '') as group_name,
		       p.cost_price, p.sell_price, p.margin_type, p.margin_value,
		       p.icon_emoji, p.sort_order, p.is_active, p.is_popular, p.stock_status,
		       COALESCE(p.is_promo, 0) as is_promo, COALESCE(p.promo_price, 0) as promo_price,
		       COALESCE(p.promo_quota, 0) as promo_quota, COALESCE(p.promo_remaining, 0) as promo_remaining,
		       p.last_sync_at, p.created_at, p.updated_at,
		       g.name as game_name, g.code as game_code
		FROM products p
		JOIN games g ON p.game_id = g.id
		LEFT JOIN product_groups pg ON p.group_id = pg.id
		WHERE p.game_id = ?
	`
	if onlyActive {
		query += " AND p.is_active = 1"
	}
	query += " ORDER BY COALESCE(pg.sort_order, 99) ASC, p.sort_order ASC, p.sell_price ASC"

	rows, err := s.db.Query(query, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []*model.Product
	for rows.Next() {
		var p model.Product
		var isActiveInt, isPopInt, isPromoInt int
		err := rows.Scan(
			&p.ID, &p.GameID, &p.DigiflazzSKU, &p.CustomSKU, &p.DisplayName, &p.DigiflazzName,
			&p.Description, &p.GroupID, &p.GroupName,
			&p.CostPrice, &p.SellPrice, &p.MarginType, &p.MarginValue,
			&p.IconEmoji, &p.SortOrder, &isActiveInt, &isPopInt, &p.StockStatus,
			&isPromoInt, &p.PromoPrice, &p.PromoQuota, &p.PromoRemaining,
			&p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
			&p.GameName, &p.GameCode,
		)
		if err != nil {
			return nil, err
		}
		p.IsActive = isActiveInt == 1
		p.IsPopular = isPopInt == 1
		p.IsPromo = isPromoInt == 1
		products = append(products, &p)
	}

	return products, nil
}

func (s *SQLiteStore) GetProductByID(id int64) (*model.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT p.id, p.game_id, p.digiflazz_sku, p.custom_sku, p.display_name, p.digiflazz_name,
		       p.description, p.group_id, COALESCE(pg.name, '') as group_name,
		       p.cost_price, p.sell_price, p.margin_type, p.margin_value,
		       p.icon_emoji, p.sort_order, p.is_active, p.is_popular, p.stock_status,
		       COALESCE(p.is_promo, 0) as is_promo, COALESCE(p.promo_price, 0) as promo_price,
		       COALESCE(p.promo_quota, 0) as promo_quota, COALESCE(p.promo_remaining, 0) as promo_remaining,
		       p.last_sync_at, p.created_at, p.updated_at,
		       g.name as game_name, g.code as game_code
		FROM products p
		JOIN games g ON p.game_id = g.id
		LEFT JOIN product_groups pg ON p.group_id = pg.id
		WHERE p.id = ?
	`
	var p model.Product
	var isActiveInt, isPopInt, isPromoInt int
	err := s.db.QueryRow(query, id).Scan(
		&p.ID, &p.GameID, &p.DigiflazzSKU, &p.CustomSKU, &p.DisplayName, &p.DigiflazzName,
		&p.Description, &p.GroupID, &p.GroupName,
		&p.CostPrice, &p.SellPrice, &p.MarginType, &p.MarginValue,
		&p.IconEmoji, &p.SortOrder, &isActiveInt, &isPopInt, &p.StockStatus,
		&isPromoInt, &p.PromoPrice, &p.PromoQuota, &p.PromoRemaining,
		&p.LastSyncAt, &p.CreatedAt, &p.UpdatedAt,
		&p.GameName, &p.GameCode,
	)
	if err != nil {
		return nil, err
	}
	p.IsActive = isActiveInt == 1
	p.IsPopular = isPopInt == 1
	p.IsPromo = isPromoInt == 1
	return &p, nil
}

// DetermineProductGroupName automatically classifies products into logical groups
func DetermineProductGroupName(gameName, productName string) string {
	lowerName := strings.ToLower(productName)
	// Check Pass / Membership / Subscription
	passKeywords := []string{"pass", "membership", "member", "starlight", "twilight", "langganan", "subscribe", "subscription", "season", "battle pass"}
	for _, kw := range passKeywords {
		if strings.Contains(lowerName, kw) {
			return "Membership & Pass"
		}
	}

	lowerGame := strings.ToLower(gameName)
	if strings.Contains(lowerGame, "mobile legends") || strings.Contains(lowerGame, "free fire") || strings.Contains(lowerName, "diamond") || strings.Contains(lowerName, "dm") {
		return "Top Up Diamond"
	}
	if strings.Contains(lowerGame, "pubg") || strings.Contains(lowerName, "uc") {
		return "Top Up UC"
	}
	if strings.Contains(lowerName, "voucher") {
		return "Voucher"
	}
	if strings.Contains(lowerName, "coin") || strings.Contains(lowerName, "koin") || strings.Contains(lowerName, "gold") {
		return "Koin & Gold"
	}
	return "Top Up Nominal"
}

func (s *SQLiteStore) getOrCreateProductGroupLocked(gameID int64, name string) (int64, error) {
	var id int64
	err := s.db.QueryRow("SELECT id FROM product_groups WHERE game_id = ? AND name = ?", gameID, name).Scan(&id)
	if err == nil {
		return id, nil
	}

	sortOrder := 10
	if name == "Membership & Pass" {
		sortOrder = 1
	} else if name == "Top Up Diamond" {
		sortOrder = 2
	} else if name == "Top Up UC" {
		sortOrder = 3
	}

	res, err := s.db.Exec("INSERT INTO product_groups (game_id, name, sort_order) VALUES (?, ?, ?)", gameID, name, sortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) GetOrCreateProductGroup(gameID int64, name string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateProductGroupLocked(gameID, name)
}

func (s *SQLiteStore) GetProductGroupsByGameID(gameID int64) ([]*model.ProductGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, game_id, name, sort_order, is_active, created_at FROM product_groups WHERE game_id = ? ORDER BY sort_order ASC, name ASC", gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*model.ProductGroup
	for rows.Next() {
		var g model.ProductGroup
		var isActiveInt int
		if err := rows.Scan(&g.ID, &g.GameID, &g.Name, &g.SortOrder, &isActiveInt, &g.CreatedAt); err == nil {
			g.IsActive = isActiveInt == 1
			groups = append(groups, &g)
		}
	}
	return groups, nil
}

func (s *SQLiteStore) AutoAssignProductGroups() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT p.id, p.game_id, p.display_name, p.digiflazz_name, g.name
		FROM products p
		JOIN games g ON p.game_id = g.id
		WHERE p.group_id IS NULL OR p.group_id = 0
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type unassignedItem struct {
		id       int64
		gameID   int64
		name     string
		gameName string
	}
	var items []unassignedItem
	for rows.Next() {
		var it unassignedItem
		var dispName, digiName string
		if err := rows.Scan(&it.id, &it.gameID, &dispName, &digiName, &it.gameName); err == nil {
			it.name = dispName
			if it.name == "" {
				it.name = digiName
			}
			items = append(items, it)
		}
	}

	for _, it := range items {
		groupName := DetermineProductGroupName(it.gameName, it.name)
		gID, err := s.getOrCreateProductGroupLocked(it.gameID, groupName)
		if err == nil && gID > 0 {
			_, _ = s.db.Exec("UPDATE products SET group_id = ? WHERE id = ?", gID, it.id)
		}
	}
	return nil
}

func (s *SQLiteStore) CreateProduct(p *model.Product) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p.GroupID == nil || *p.GroupID == 0 {
		var gameName string
		_ = s.db.QueryRow("SELECT name FROM games WHERE id = ?", p.GameID).Scan(&gameName)
		nameToUse := p.DisplayName
		if nameToUse == "" {
			nameToUse = p.DigiflazzName
		}
		groupName := DetermineProductGroupName(gameName, nameToUse)
		gID, err := s.getOrCreateProductGroupLocked(p.GameID, groupName)
		if err == nil && gID > 0 {
			p.GroupID = &gID
		}
	}

	res, err := s.db.Exec(`
		INSERT INTO products (
			game_id, digiflazz_sku, custom_sku, display_name, digiflazz_name,
			description, group_id, cost_price, sell_price, margin_type, margin_value,
			icon_emoji, sort_order, is_active, is_popular, stock_status, last_sync_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, p.GameID, p.DigiflazzSKU, p.CustomSKU, p.DisplayName, p.DigiflazzName,
		p.Description, p.GroupID, p.CostPrice, p.SellPrice, p.MarginType, p.MarginValue,
		p.IconEmoji, p.SortOrder, boolToInt(p.IsActive), boolToInt(p.IsPopular), p.StockStatus)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) UpdateProduct(p *model.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE products SET
			digiflazz_sku = ?, custom_sku = ?, display_name = ?, description = ?,
			group_id = ?, cost_price = ?, sell_price = ?,
			margin_type = ?, margin_value = ?, icon_emoji = ?,
			sort_order = ?, is_active = ?, is_popular = ?,
			is_promo = ?, promo_price = ?, promo_quota = ?, promo_remaining = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, p.DigiflazzSKU, p.CustomSKU, p.DisplayName, p.Description, p.GroupID, p.CostPrice, p.SellPrice,
		p.MarginType, p.MarginValue, p.IconEmoji, p.SortOrder, boolToInt(p.IsActive), boolToInt(p.IsPopular),
		boolToInt(p.IsPromo), p.PromoPrice, p.PromoQuota, p.PromoRemaining, p.ID)
	return err
}

func (s *SQLiteStore) DeductProductPromoQuota(productID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE products
		SET promo_remaining = CASE WHEN promo_remaining > 0 THEN promo_remaining - 1 ELSE 0 END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND is_promo = 1
	`, productID)
	return err
}

func (s *SQLiteStore) ToggleProduct(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`UPDATE products SET is_active = 1 - is_active, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) DeleteProduct(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = s.db.Exec("PRAGMA foreign_keys = OFF")
	defer func() {
		_, _ = s.db.Exec("PRAGMA foreign_keys = ON")
	}()

	_, err := s.db.Exec("DELETE FROM products WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) BulkToggleProducts(gameID int64, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE products SET is_active = ?, updated_at = CURRENT_TIMESTAMP WHERE game_id = ?", boolToInt(active), gameID)
	return err
}

func (s *SQLiteStore) BulkMarginProducts(gameID int64, marginType string, marginVal int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT id, cost_price FROM products WHERE game_id = ?", gameID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		id   int64
		cost int
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.cost); err == nil {
			items = append(items, it)
		}
	}

	for _, it := range items {
		sell := model.CalculateSellPrice(it.cost, marginType, marginVal)
		_, _ = s.db.Exec(`
			UPDATE products SET margin_type = ?, margin_value = ?, sell_price = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, marginType, marginVal, sell, it.id)
	}

	return nil
}

func (s *SQLiteStore) UpsertDigiflazzPriceSync(sku string, costPrice int, stockStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Select product by digiflazz_sku
	rows, err := s.db.Query("SELECT id, margin_type, margin_value, sell_price FROM products WHERE digiflazz_sku = ?", sku)
	if err != nil {
		return err
	}
	defer rows.Close()

	type prodInfo struct {
		id          int64
		marginType  string
		marginValue int
		sellPrice   int
	}
	var prods []prodInfo
	for rows.Next() {
		var p prodInfo
		if err := rows.Scan(&p.id, &p.marginType, &p.marginValue, &p.sellPrice); err == nil {
			prods = append(prods, p)
		}
	}

	now := time.Now()
	for _, p := range prods {
		newSell := p.sellPrice
		if p.marginType == "fixed" || p.marginType == "percent" {
			newSell = model.CalculateSellPrice(costPrice, p.marginType, p.marginValue)
		}
		_, _ = s.db.Exec(`
			UPDATE products SET
				cost_price = ?,
				sell_price = ?,
				stock_status = ?,
				last_sync_at = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, costPrice, newSell, stockStatus, now, p.id)
	}

	return nil
}

// ================= ORDER STORE =================

func (s *SQLiteStore) CreateOrder(o *model.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return insertOrder(s.db, o)
}

// sqlExecer is satisfied by both *sql.DB and *sql.Tx.
type sqlExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func insertOrder(db sqlExecer, o *model.Order) error {
	if o.PaymentMethod == "" {
		o.PaymentMethod = model.PaymentMethodQRIS
	}
	_, err := db.Exec(`
		INSERT INTO orders (
			id, game_id, customer_no, customer_no2, customer_email, product_id, product_name,
			price, cost, status, payment_trx_id, payment_checkout_url, payment_qr_url,
			payment_qr_string, payment_expiry, digiflazz_ref_id, digiflazz_sn,
			digiflazz_status, digiflazz_message, admin_note, user_id, payment_method, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, o.ID, o.GameID, o.CustomerNo, o.CustomerNo2, o.CustomerEmail, o.ProductID, o.ProductName,
		o.Price, o.Cost, o.Status, o.PaymentTrxID, o.PaymentCheckoutURL, o.PaymentQRURL,
		o.PaymentQRString, o.PaymentExpiry, o.DigiflazzRefID, o.DigiflazzSN,
		o.DigiflazzStatus, o.DigiflazzMessage, o.AdminNote, o.UserID, o.PaymentMethod)
	return err
}

func (s *SQLiteStore) GetOrderByID(id string) (*model.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT o.id, o.game_id, o.customer_no, o.customer_no2, o.customer_email, o.product_id, o.product_name,
		       o.price, o.cost, o.status, o.payment_trx_id, o.payment_checkout_url, o.payment_qr_url,
		       o.payment_qr_string, o.payment_expiry, o.digiflazz_ref_id, o.digiflazz_sn,
		       o.digiflazz_status, o.digiflazz_message, o.admin_note, o.created_at, o.updated_at,
		       g.name as game_name, g.code as game_code,
		       COALESCE(o.user_id, 0), COALESCE(o.payment_method, 'qris')
		FROM orders o
		JOIN games g ON o.game_id = g.id
		WHERE o.id = ?
	`
	var o model.Order
	err := s.db.QueryRow(query, id).Scan(
		&o.ID, &o.GameID, &o.CustomerNo, &o.CustomerNo2, &o.CustomerEmail, &o.ProductID, &o.ProductName,
		&o.Price, &o.Cost, &o.Status, &o.PaymentTrxID, &o.PaymentCheckoutURL, &o.PaymentQRURL,
		&o.PaymentQRString, &o.PaymentExpiry, &o.DigiflazzRefID, &o.DigiflazzSN,
		&o.DigiflazzStatus, &o.DigiflazzMessage, &o.AdminNote, &o.CreatedAt, &o.UpdatedAt,
		&o.GameName, &o.GameCode, &o.UserID, &o.PaymentMethod,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *SQLiteStore) GetOrderByPaymentTrxID(trxID string) (*model.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT o.id, o.game_id, o.customer_no, o.customer_no2, o.customer_email, o.product_id, o.product_name,
		       o.price, o.cost, o.status, o.payment_trx_id, o.payment_checkout_url, o.payment_qr_url,
		       o.payment_qr_string, o.payment_expiry, o.digiflazz_ref_id, o.digiflazz_sn,
		       o.digiflazz_status, o.digiflazz_message, o.admin_note, o.created_at, o.updated_at,
		       g.name as game_name, g.code as game_code
		FROM orders o
		JOIN games g ON o.game_id = g.id
		WHERE o.payment_trx_id = ?
	`
	var o model.Order
	err := s.db.QueryRow(query, trxID).Scan(
		&o.ID, &o.GameID, &o.CustomerNo, &o.CustomerNo2, &o.CustomerEmail, &o.ProductID, &o.ProductName,
		&o.Price, &o.Cost, &o.Status, &o.PaymentTrxID, &o.PaymentCheckoutURL, &o.PaymentQRURL,
		&o.PaymentQRString, &o.PaymentExpiry, &o.DigiflazzRefID, &o.DigiflazzSN,
		&o.DigiflazzStatus, &o.DigiflazzMessage, &o.AdminNote, &o.CreatedAt, &o.UpdatedAt,
		&o.GameName, &o.GameCode,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *SQLiteStore) GetOrderByDigiflazzRefID(refID string) (*model.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT o.id, o.game_id, o.customer_no, o.customer_no2, o.customer_email, o.product_id, o.product_name,
		       o.price, o.cost, o.status, o.payment_trx_id, o.payment_checkout_url, o.payment_qr_url,
		       o.payment_qr_string, o.payment_expiry, o.digiflazz_ref_id, o.digiflazz_sn,
		       o.digiflazz_status, o.digiflazz_message, o.admin_note, o.created_at, o.updated_at,
		       g.name as game_name, g.code as game_code
		FROM orders o
		JOIN games g ON o.game_id = g.id
		WHERE o.digiflazz_ref_id = ?
	`
	var o model.Order
	err := s.db.QueryRow(query, refID).Scan(
		&o.ID, &o.GameID, &o.CustomerNo, &o.CustomerNo2, &o.CustomerEmail, &o.ProductID, &o.ProductName,
		&o.Price, &o.Cost, &o.Status, &o.PaymentTrxID, &o.PaymentCheckoutURL, &o.PaymentQRURL,
		&o.PaymentQRString, &o.PaymentExpiry, &o.DigiflazzRefID, &o.DigiflazzSN,
		&o.DigiflazzStatus, &o.DigiflazzMessage, &o.AdminNote, &o.CreatedAt, &o.UpdatedAt,
		&o.GameName, &o.GameCode,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *SQLiteStore) UpdateOrderStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`UPDATE orders SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id)
	return err
}

func (s *SQLiteStore) UpdateOrderPayment(id, trxID, checkoutURL, qrURL, qrString string, expiry *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE orders SET
			payment_trx_id = ?,
			payment_checkout_url = ?,
			payment_qr_url = ?,
			payment_qr_string = ?,
			payment_expiry = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, trxID, checkoutURL, qrURL, qrString, expiry, id)
	return err
}

func (s *SQLiteStore) UpdateOrderDigiflazz(id, refID, status, sn, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE orders SET
			digiflazz_ref_id = ?,
			digiflazz_status = ?,
			digiflazz_sn = ?,
			digiflazz_message = ?,
			status = CASE WHEN ? = 'success' THEN 'success' WHEN ? = 'failed' THEN 'failed' ELSE status END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, refID, status, sn, msg, status, status, id)
	return err
}

func (s *SQLiteStore) UpdateOrderNote(id, note, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE orders SET admin_note = ?, updated_at = CURRENT_TIMESTAMP`
	args := []interface{}{note}
	if status != "" {
		query += `, status = ?`
		args = append(args, status)
	}
	query += ` WHERE id = ?`
	args = append(args, id)

	_, err := s.db.Exec(query, args...)
	return err
}

func (s *SQLiteStore) GetRecentOrders(limit int) ([]*model.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT o.id, o.game_id, o.customer_no, o.customer_no2, o.customer_email, o.product_id, o.product_name,
		       o.price, o.cost, o.status, o.payment_trx_id, o.payment_checkout_url, o.payment_qr_url,
		       o.payment_qr_string, o.payment_expiry, o.digiflazz_ref_id, o.digiflazz_sn,
		       o.digiflazz_status, o.digiflazz_message, o.admin_note, o.created_at, o.updated_at,
		       g.name as game_name, g.code as game_code
		FROM orders o
		JOIN games g ON o.game_id = g.id
		ORDER BY o.created_at DESC
		LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*model.Order
	for rows.Next() {
		var o model.Order
		err := rows.Scan(
			&o.ID, &o.GameID, &o.CustomerNo, &o.CustomerNo2, &o.CustomerEmail, &o.ProductID, &o.ProductName,
			&o.Price, &o.Cost, &o.Status, &o.PaymentTrxID, &o.PaymentCheckoutURL, &o.PaymentQRURL,
			&o.PaymentQRString, &o.PaymentExpiry, &o.DigiflazzRefID, &o.DigiflazzSN,
			&o.DigiflazzStatus, &o.DigiflazzMessage, &o.AdminNote, &o.CreatedAt, &o.UpdatedAt,
			&o.GameName, &o.GameCode,
		)
		if err != nil {
			return nil, err
		}
		orders = append(orders, &o)
	}

	return orders, nil
}

func (s *SQLiteStore) GetAllOrders(statusFilter, gameFilter, search string, limit, offset int) ([]*model.Order, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	whereClauses := []string{"1=1"}
	var args []interface{}

	if statusFilter != "" {
		whereClauses = append(whereClauses, "o.status = ?")
		args = append(args, statusFilter)
	}
	if gameFilter != "" {
		whereClauses = append(whereClauses, "g.code = ?")
		args = append(args, gameFilter)
	}
	if search != "" {
		whereClauses = append(whereClauses, "(o.id LIKE ? OR o.customer_no LIKE ? OR o.customer_email LIKE ? OR o.digiflazz_sn LIKE ?)")
		sTerm := "%" + search + "%"
		args = append(args, sTerm, sTerm, sTerm, sTerm)
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM orders o JOIN games g ON o.game_id = g.id WHERE %s", whereSQL)
	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Data query
	dataQuery := fmt.Sprintf(`
		SELECT o.id, o.game_id, o.customer_no, o.customer_no2, o.customer_email, o.product_id, o.product_name,
		       o.price, o.cost, o.status, o.payment_trx_id, o.payment_checkout_url, o.payment_qr_url,
		       o.payment_qr_string, o.payment_expiry, o.digiflazz_ref_id, o.digiflazz_sn,
		       o.digiflazz_status, o.digiflazz_message, o.admin_note, o.created_at, o.updated_at,
		       g.name as game_name, g.code as game_code,
		       COALESCE(o.user_id, 0), COALESCE(o.payment_method, 'qris')
		FROM orders o
		JOIN games g ON o.game_id = g.id
		WHERE %s
		ORDER BY o.created_at DESC
		LIMIT ? OFFSET ?
	`, whereSQL)

	qArgs := append(args, limit, offset)
	rows, err := s.db.Query(dataQuery, qArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []*model.Order
	for rows.Next() {
		var o model.Order
		err := rows.Scan(
			&o.ID, &o.GameID, &o.CustomerNo, &o.CustomerNo2, &o.CustomerEmail, &o.ProductID, &o.ProductName,
			&o.Price, &o.Cost, &o.Status, &o.PaymentTrxID, &o.PaymentCheckoutURL, &o.PaymentQRURL,
			&o.PaymentQRString, &o.PaymentExpiry, &o.DigiflazzRefID, &o.DigiflazzSN,
			&o.DigiflazzStatus, &o.DigiflazzMessage, &o.AdminNote, &o.CreatedAt, &o.UpdatedAt,
			&o.GameName, &o.GameCode, &o.UserID, &o.PaymentMethod,
		)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, &o)
	}

	return orders, total, nil
}

func (s *SQLiteStore) ExpireOldPendingOrders() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	res, err := s.db.Exec(`
		UPDATE orders SET status = 'expired', updated_at = CURRENT_TIMESTAMP
		WHERE status = 'pending_payment' AND payment_expiry IS NOT NULL AND payment_expiry < ?
	`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteStore) GetProcessingOrders() ([]*model.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT o.id, o.game_id, o.customer_no, o.customer_no2, o.customer_email, o.product_id, o.product_name,
		       o.price, o.cost, o.status, o.payment_trx_id, o.payment_checkout_url, o.payment_qr_url,
		       o.payment_qr_string, o.payment_expiry, o.digiflazz_ref_id, o.digiflazz_sn,
		       o.digiflazz_status, o.digiflazz_message, o.admin_note, o.created_at, o.updated_at,
		       g.name as game_name, g.code as game_code
		FROM orders o
		JOIN games g ON o.game_id = g.id
		WHERE o.status = 'processing'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*model.Order
	for rows.Next() {
		var o model.Order
		err := rows.Scan(
			&o.ID, &o.GameID, &o.CustomerNo, &o.CustomerNo2, &o.CustomerEmail, &o.ProductID, &o.ProductName,
			&o.Price, &o.Cost, &o.Status, &o.PaymentTrxID, &o.PaymentCheckoutURL, &o.PaymentQRURL,
			&o.PaymentQRString, &o.PaymentExpiry, &o.DigiflazzRefID, &o.DigiflazzSN,
			&o.DigiflazzStatus, &o.DigiflazzMessage, &o.AdminNote, &o.CreatedAt, &o.UpdatedAt,
			&o.GameName, &o.GameCode,
		)
		if err != nil {
			return nil, err
		}
		orders = append(orders, &o)
	}

	return orders, nil
}

// ================= ADMIN & SESSION =================

func (s *SQLiteStore) GetAdminByUsername(username string) (*model.AdminUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var a model.AdminUser
	var isActiveInt int
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, display_name, role, is_active, last_login_at, created_at
		FROM admin_users WHERE username = ?
	`, username).Scan(
		&a.ID, &a.Username, &a.PasswordHash, &a.DisplayName, &a.Role, &isActiveInt, &a.LastLoginAt, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.IsActive = isActiveInt == 1
	return &a, nil
}

func (s *SQLiteStore) UpdateAdminLogin(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE admin_users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) CreateSession(token string, adminID int64, ip string, maxAgeSec int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiresAt := time.Now().Add(time.Duration(maxAgeSec) * time.Second)
	_, err := s.db.Exec(`
		INSERT INTO admin_sessions (token, admin_id, ip_address, expires_at, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, token, adminID, ip, expiresAt)
	return err
}

func (s *SQLiteStore) GetSession(token string) (*model.AdminSession, *model.AdminUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT s.token, s.admin_id, s.ip_address, s.expires_at, s.created_at,
		       u.id, u.username, u.display_name, u.role, u.is_active, u.created_at
		FROM admin_sessions s
		JOIN admin_users u ON s.admin_id = u.id
		WHERE s.token = ? AND s.expires_at > CURRENT_TIMESTAMP AND u.is_active = 1
	`
	var sess model.AdminSession
	var user model.AdminUser
	var isActiveInt int
	err := s.db.QueryRow(query, token).Scan(
		&sess.Token, &sess.AdminID, &sess.IPAddress, &sess.ExpiresAt, &sess.CreatedAt,
		&user.ID, &user.Username, &user.DisplayName, &user.Role, &isActiveInt, &user.CreatedAt,
	)
	if err != nil {
		return nil, nil, err
	}
	user.IsActive = isActiveInt == 1
	return &sess, &user, nil
}

func (s *SQLiteStore) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM admin_sessions WHERE token = ?", token)
	return err
}

func (s *SQLiteStore) CleanExpiredSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM admin_sessions WHERE expires_at < CURRENT_TIMESTAMP")
	return err
}

func (s *SQLiteStore) CreateAuditLog(adminID int64, action, entityType, entityID, oldValue, newValue string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO audit_log (admin_id, action, entity_type, entity_id, old_value, new_value, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, adminID, action, entityType, entityID, oldValue, newValue)
	return err
}

func (s *SQLiteStore) GetAuditLogs(limit int) ([]*model.AuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT l.id, l.admin_id, COALESCE(u.username, 'System') as admin_username,
		       l.action, l.entity_type, l.entity_id, l.old_value, l.new_value, l.created_at
		FROM audit_log l
		LEFT JOIN admin_users u ON l.admin_id = u.id
		ORDER BY l.created_at DESC
		LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*model.AuditLog
	for rows.Next() {
		var log model.AuditLog
		err := rows.Scan(
			&log.ID, &log.AdminID, &log.AdminUsername,
			&log.Action, &log.EntityType, &log.EntityID,
			&log.OldValue, &log.NewValue, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, nil
}

func (s *SQLiteStore) GetDashboardStats() (*model.DashboardStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &model.DashboardStats{}

	// Today order stats (start of today in local time)
	todayStr := time.Now().Format("2006-01-02")
	_ = s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(price), 0), COALESCE(SUM(price - cost), 0)
		FROM orders
		WHERE DATE(created_at) = DATE(?) AND status = 'success'
	`, todayStr).Scan(&stats.TodayOrderCount, &stats.TodayRevenue, &stats.TodayProfit)

	// Counts
	_ = s.db.QueryRow("SELECT COUNT(*) FROM games WHERE is_active = 1").Scan(&stats.ActiveGameCount)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM products WHERE is_active = 1").Scan(&stats.ActiveProductCount)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM orders WHERE status = 'pending_payment'").Scan(&stats.PendingOrderCount)

	return stats, nil
}

func (s *SQLiteStore) GetAllAdmins() ([]*model.AdminUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, username, display_name, role, is_active, last_login_at, created_at
		FROM admin_users ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []*model.AdminUser
	for rows.Next() {
		var a model.AdminUser
		var isActiveInt int
		if err := rows.Scan(&a.ID, &a.Username, &a.DisplayName, &a.Role, &isActiveInt, &a.LastLoginAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.IsActive = isActiveInt == 1
		admins = append(admins, &a)
	}
	return admins, nil
}

func (s *SQLiteStore) CreateAdmin(username, password, displayName, role string) (*model.AdminUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	if role != "superadmin" {
		role = "admin"
	}

	res, err := s.db.Exec(`
		INSERT INTO admin_users (username, password_hash, display_name, role, is_active, created_at)
		VALUES (?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
	`, username, string(hash), displayName, role)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &model.AdminUser{
		ID:          id,
		Username:    username,
		DisplayName: displayName,
		Role:        role,
		IsActive:    true,
		CreatedAt:   time.Now(),
	}, nil
}

func (s *SQLiteStore) UpdateAdmin(id int64, displayName, role string, isActive bool, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	isActiveInt := 0
	if isActive {
		isActiveInt = 1
	}

	if role != "superadmin" {
		role = "admin"
	}

	if newPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(`
			UPDATE admin_users
			SET display_name = ?, role = ?, is_active = ?, password_hash = ?
			WHERE id = ?
		`, displayName, role, isActiveInt, string(hash), id)
		return err
	}

	_, err := s.db.Exec(`
		UPDATE admin_users
		SET display_name = ?, role = ?, is_active = ?
		WHERE id = ?
	`, displayName, role, isActiveInt, id)
	return err
}

func (s *SQLiteStore) ToggleAdminActive(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE admin_users SET is_active = CASE WHEN is_active = 1 THEN 0 ELSE 1 END WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) DeleteAdmin(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM admin_users WHERE id = ?", id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
