package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"topupku/handler"
	"topupku/handler/admin"
	"topupku/middleware"
	"topupku/model"
	"topupku/service"
	"topupku/store"
)

func setupTestApp(t *testing.T) (*http.ServeMux, *store.SQLiteStore, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "topup_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to init sqlite store: %v", err)
	}

	if err := st.SeedInitialData("admin", "admin123"); err != nil {
		t.Fatalf("Failed to seed initial data: %v", err)
	}

	dfClient := service.NewDigiflazzClient("", "")
	agpClient := service.NewAutoGoPayClient("", "")
	emailSvc := service.NewEmailService("", "", "", "", "", "", "http://localhost:8080")

	orderSvc := service.NewOrderService(st, agpClient, dfClient, emailSvc)
	adminSvc := service.NewAdminService(st, dfClient)

	pageHandler := handler.NewPageHandler(st, orderSvc, true)
	orderHandler := handler.NewOrderHandler(st, orderSvc)

	dashHandler := admin.NewDashboardHandler(st, dfClient)
	gameHandler := admin.NewGameAdminHandler(st)
	prodHandler := admin.NewProductAdminHandler(st)
	orderAdminHandler := admin.NewOrderAdminHandler(st, orderSvc)
	auditHandler := admin.NewAuditAdminHandler(st)
	authHandler := admin.NewAuthHandler(st, adminSvc, 86400)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", pageHandler.PageHome)
	mux.HandleFunc("GET /game/{code}", pageHandler.PageGame)
	mux.HandleFunc("GET /order/{id}", pageHandler.PageOrderStatus)

	mux.HandleFunc("POST /api/order", orderHandler.CreateOrder)
	mux.HandleFunc("GET /api/order/{id}/status", orderHandler.GetOrderStatus)
	mux.HandleFunc("POST /api/order/{id}/simulate-pay", orderHandler.SimulatePayment)

	// Admin routes
	requireAuth := middleware.RequireAdmin(st)
	mux.HandleFunc("GET /admin/login", authHandler.LoginPage)
	mux.HandleFunc("POST /admin/login", authHandler.LoginSubmit)
	mux.Handle("GET /admin/dashboard", requireAuth(http.HandlerFunc(dashHandler.Dashboard)))
	mux.Handle("GET /admin/games", requireAuth(http.HandlerFunc(gameHandler.GameList)))
	mux.Handle("GET /admin/games/{id}/products", requireAuth(http.HandlerFunc(prodHandler.ProductList)))
	mux.Handle("GET /admin/products/{id}/edit", requireAuth(http.HandlerFunc(prodHandler.ProductForm)))
	mux.Handle("GET /admin/orders", requireAuth(http.HandlerFunc(orderAdminHandler.OrderList)))
	mux.Handle("GET /admin/audit-log", requireAuth(http.HandlerFunc(auditHandler.AuditLog)))

	cleanup := func() {
		st.Close()
		os.RemoveAll(tmpDir)
	}

	return mux, st, cleanup
}

func TestHomePage(t *testing.T) {
	mux, _, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for home page, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Azeotopup") {
		t.Errorf("Expected body to contain Azeotopup")
	}
}

func TestGamePage(t *testing.T) {
	mux, _, cleanup := setupTestApp(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/game/ml", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for game page /game/ml, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Mobile Legends") {
		t.Errorf("Expected body to contain Mobile Legends")
	}
}

func TestOrderFlow(t *testing.T) {
	mux, st, cleanup := setupTestApp(t)
	defer cleanup()

	// Get games & products
	games, err := st.GetAllGames(true)
	if err != nil || len(games) == 0 {
		t.Fatalf("Failed to get games: %v", err)
	}
	game := games[0]

	products, err := st.GetProductsByGameID(game.ID, true)
	if err != nil || len(products) == 0 {
		t.Fatalf("Failed to get products: %v", err)
	}
	product := products[0]

	// 1. Create order
	reqBody, _ := json.Marshal(map[string]interface{}{
		"game_code":      game.Code,
		"customer_no":    "12345678",
		"customer_no2":   "2134",
		"customer_email": "tester@example.com",
		"product_id":     product.ID,
	})

	req := httptest.NewRequest("POST", "/api/order", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for create order, got %d: %s", rr.Code, rr.Body.String())
	}

	var res struct {
		Success bool `json:"success"`
		Data    struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to parse create order response: %v", err)
	}
	if !res.Success || res.Data.OrderID == "" {
		t.Fatalf("Order creation failed: %v", rr.Body.String())
	}

	orderID := res.Data.OrderID

	// 2. View order status page
	reqStatus := httptest.NewRequest("GET", "/order/"+orderID, nil)
	rrStatus := httptest.NewRecorder()
	mux.ServeHTTP(rrStatus, reqStatus)
	if rrStatus.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for order status page, got %d: %s", rrStatus.Code, rrStatus.Body.String())
	}

	// 3. Check API status
	reqAPIStatus := httptest.NewRequest("GET", "/api/order/"+orderID+"/status", nil)
	rrAPIStatus := httptest.NewRecorder()
	mux.ServeHTTP(rrAPIStatus, reqAPIStatus)
	if rrAPIStatus.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for API status, got %d: %s", rrAPIStatus.Code, rrAPIStatus.Body.String())
	}

	// 4. Simulate Payment
	reqSim := httptest.NewRequest("POST", "/api/order/"+orderID+"/simulate-pay", nil)
	rrSim := httptest.NewRecorder()
	mux.ServeHTTP(rrSim, reqSim)
	if rrSim.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for simulate payment, got %d: %s", rrSim.Code, rrSim.Body.String())
	}

	// 5. Verify order is paid/success
	order, err := st.GetOrderByID(orderID)
	if err != nil {
		t.Fatalf("Failed to fetch order after simulation: %v", err)
	}
	if order.Status != model.StatusSuccess && order.Status != model.StatusPaid && order.Status != model.StatusProcessing {
		t.Errorf("Unexpected order status after pay simulation: %s", order.Status)
	}
}

func TestMandatoryEmail(t *testing.T) {
	mux, st, cleanup := setupTestApp(t)
	defer cleanup()

	games, _ := st.GetAllGames(true)
	game := games[0]
	products, _ := st.GetProductsByGameID(game.ID, true)
	product := products[0]

	// Order without email should fail
	reqNoEmail, _ := json.Marshal(map[string]interface{}{
		"game_code":      game.Code,
		"customer_no":    "12345678",
		"customer_no2":   "2134",
		"customer_email": "",
		"product_id":     product.ID,
	})

	req := httptest.NewRequest("POST", "/api/order", bytes.NewReader(reqNoEmail))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request when email is missing, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "email wajib diisi") {
		t.Errorf("Expected error message about email, got: %s", rr.Body.String())
	}
}

func TestAdminPages(t *testing.T) {
	mux, st, cleanup := setupTestApp(t)
	defer cleanup()

	// Create admin session
	adminUser, err := st.GetAdminByUsername("admin")
	if err != nil {
		t.Fatalf("Failed to get admin: %v", err)
	}
	token := "test-session-token-1234567890123456789012345678901234567890"
	err = st.CreateSession(token, adminUser.ID, "127.0.0.1", 3600)
	if err != nil {
		t.Fatalf("Failed to create admin session: %v", err)
	}
	sessionCookie := &http.Cookie{
		Name:  "admin_session",
		Value: token,
	}

	adminPaths := []string{
		"/admin/dashboard",
		"/admin/games",
		"/admin/games/1/products",
		"/admin/products/1/edit",
		"/admin/orders",
		"/admin/audit-log",
	}

	for _, p := range adminPaths {
		req := httptest.NewRequest("GET", p, nil)
		req.AddCookie(sessionCookie)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Path %s failed with code %d: %s", p, rr.Code, rr.Body.String())
		}
	}
}

func TestFlashSalePromoFlow(t *testing.T) {
	mux, st, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Set product 1 as promo
	prod, err := st.GetProductByID(1)
	if err != nil {
		t.Fatalf("Failed to get product: %v", err)
	}

	prod.IsPromo = true
	prod.PromoPrice = 15000
	prod.PromoQuota = 5
	prod.PromoRemaining = 5
	if err := st.UpdateProduct(prod); err != nil {
		t.Fatalf("Failed to update product promo: %v", err)
	}

	// 2. Test Game Page rendering with Promo
	req := httptest.NewRequest("GET", "/game/ml", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Failed to render game page with promo: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "FLASH SALE") {
		t.Errorf("Expected page to contain 'FLASH SALE'")
	}
	if !strings.Contains(body, "5 Slot") {
		t.Errorf("Expected page to contain '5 Slot'")
	}

	// 3. Create order for promo product
	reqBody, _ := json.Marshal(map[string]interface{}{
		"game_code":      "ml",
		"customer_no":    "12345678",
		"customer_no2":   "2134",
		"customer_email": "promo@example.com",
		"product_id":     1,
	})

	reqOrder := httptest.NewRequest("POST", "/api/order", bytes.NewReader(reqBody))
	reqOrder.Header.Set("Content-Type", "application/json")
	rrOrder := httptest.NewRecorder()
	mux.ServeHTTP(rrOrder, reqOrder)

	var res struct {
		Success bool `json:"success"`
		Data    struct {
			OrderID string `json:"order_id"`
			Price   int    `json:"price"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rrOrder.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if res.Data.Price != 15000 {
		t.Errorf("Expected promo price 15000, got %d", res.Data.Price)
	}

	// 4. Simulate payment and verify quota reduction
	reqSim := httptest.NewRequest("POST", "/api/order/"+res.Data.OrderID+"/simulate-pay", nil)
	rrSim := httptest.NewRecorder()
	mux.ServeHTTP(rrSim, reqSim)

	updatedProd, err := st.GetProductByID(1)
	if err != nil {
		t.Fatalf("Failed to get updated product: %v", err)
	}
	if updatedProd.PromoRemaining != 4 {
		t.Errorf("Expected promo_remaining to be 4, got %d", updatedProd.PromoRemaining)
	}
}

func TestAllRemainingTemplates(t *testing.T) {
	_, _, cleanup := setupTestApp(t)
	defer cleanup()

	// Test directly parsing and rendering all templates
	templatesToTest := []struct {
		name       string
		layout     string
		pages      []string
		sampleData interface{}
	}{
		{
			name:   "Admin Login",
			layout: "template/admin/layout.html",
			pages:  []string{"template/admin/login.html"},
			sampleData: map[string]interface{}{
				"Title": "Login Admin",
				"Error": "",
			},
		},
		{
			name:   "Admin Game Form (Add)",
			layout: "template/admin/layout.html",
			pages:  []string{"template/admin/game_form.html"},
			sampleData: map[string]interface{}{
				"Title":     "Tambah Game",
				"ActiveTab": "games",
				"Game":      &model.Game{},
			},
		},
		{
			name:   "Admin Product Import",
			layout: "template/admin/layout.html",
			pages:  []string{"template/admin/product_import.html"},
			sampleData: map[string]interface{}{
				"Title":     "Import Produk",
				"ActiveTab": "games",
				"Game":      &model.Game{ID: 1, Name: "Mobile Legends"},
				"ImportList": []map[string]interface{}{
					{
						"BuyerSKUCode": "ml-86",
						"ProductName":  "86 Diamonds",
						"Price":        18000,
						"IsImported":   false,
					},
				},
				"TotalFound": 1,
			},
		},
		{
			name:   "Admin Order Detail",
			layout: "template/admin/layout.html",
			pages:  []string{"template/admin/order_detail.html"},
			sampleData: map[string]interface{}{
				"Title":     "Detail Order",
				"ActiveTab": "orders",
				"Order": &model.Order{
					ID:          "TK-TEST",
					GameName:    "Mobile Legends",
					ProductName: "86 Diamonds",
					Price:       20000,
					Status:      "success",
				},
				"Logs": []*model.AuditLog{},
			},
		},
	}

	for _, tt := range templatesToTest {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.RenderTemplate(rec, tt.layout, tt.pages, tt.sampleData)
			if rec.Code != http.StatusOK {
				t.Fatalf("Failed to render %s: status %d, body: %s", tt.name, rec.Code, rec.Body.String())
			}
		})
	}
}
