package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"topupku/config"
	"topupku/handler"
	"topupku/handler/admin"
	"topupku/middleware"
	"topupku/service"
	"topupku/store"
)

func main() {
	cfg := config.LoadConfig()

	log.Printf("Starting Azeotopup Web Top-Up Service...")
	log.Printf("Port: %s, DB: %s", cfg.Port, cfg.DBPath)

	// Init SQLite Store
	st, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Fatal: Cannot initialize database: %v", err)
	}
	defer st.Close()

	// Seed initial admin user and default games/products
	if err := st.SeedInitialData(cfg.AdminInitialUsername, cfg.AdminInitialPassword); err != nil {
		log.Printf("Warning during initial seeding: %v", err)
	}

	// Clients
	dfClient := service.NewDigiflazzClient(cfg.DigiflazzUsername, cfg.DigiflazzAPIKey)
	agpClient := service.NewAutoGoPayClient(cfg.AutoGoPayAPIKey, cfg.AutoGoPayBaseURL)

	if !dfClient.IsConfigured() {
		log.Printf("[Digiflazz] Credentials not set in .env. Running in SIMULATION mode.")
	} else {
		log.Printf("[Digiflazz] CONNECTED! Username: %s (LIVE API mode enabled).", dfClient.Username)
	}
	if !agpClient.IsConfigured() {
		log.Printf("[AutoGoPay] Credentials not set in .env. Running in SIMULATION mode.")
	} else {
		log.Printf("[AutoGoPay] CONNECTED! (LIVE QRIS mode enabled).")
	}

	emailSvc := service.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom, cfg.SMTPFromName, cfg.BaseURL)
	if emailSvc.IsConfigured() {
		log.Printf("[Email] SMTP configured (%s:%s). Live receipt sending enabled.", emailSvc.Host, emailSvc.Port)
	} else {
		log.Printf("[Email] SMTP not configured. Running in SIMULATION mode (receipt logged).")
	}

	// Services
	orderSvc := service.NewOrderService(st, agpClient, dfClient, emailSvc)
	adminSvc := service.NewAdminService(st, dfClient)

	// Public Handlers
	pageHandler := handler.NewPageHandler(st, orderSvc, !agpClient.IsConfigured())
	orderHandler := handler.NewOrderHandler(st, orderSvc)
	webhookAGP := handler.NewAutoGoPayWebhookHandler(orderSvc, agpClient)
	webhookDF := handler.NewDigiflazzWebhookHandler(orderSvc, cfg.DigiflazzWebhookSecret)

	// Admin Handlers
	authHandler := admin.NewAuthHandler(st, adminSvc, cfg.AdminSessionMaxAge)
	dashHandler := admin.NewDashboardHandler(st, dfClient)
	gameHandler := admin.NewGameAdminHandler(st)
	prodHandler := admin.NewProductAdminHandler(st)
	syncHandler := admin.NewSyncAdminHandler(st, adminSvc, dfClient)
	orderAdminHandler := admin.NewOrderAdminHandler(st, orderSvc)
	auditHandler := admin.NewAuditAdminHandler(st)

	// Middleware
	requireAuth := middleware.RequireAdmin(st)

	// Routing with standard Go 1.22+ ServeMux
	mux := http.NewServeMux()

	// Static & Upload files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	// Public Pages
	mux.HandleFunc("GET /", pageHandler.PageHome)
	mux.HandleFunc("GET /game/{code}", pageHandler.PageGame)
	mux.HandleFunc("GET /order/{id}", pageHandler.PageOrderStatus)

	// Public APIs
	mux.HandleFunc("POST /api/order", orderHandler.CreateOrder)
	mux.HandleFunc("GET /api/order/{id}/status", orderHandler.GetOrderStatus)
	mux.HandleFunc("POST /api/order/{id}/simulate-pay", orderHandler.SimulatePayment)

	// Webhooks
	mux.Handle("POST /webhook/autogopay", webhookAGP)
	mux.Handle("POST /webhook/digiflazz", webhookDF)

	// Admin Auth
	mux.HandleFunc("GET /admin/login", authHandler.LoginPage)
	mux.HandleFunc("POST /admin/login", authHandler.LoginSubmit)
	mux.HandleFunc("POST /admin/logout", authHandler.Logout)

	// Admin Dashboard & Protected Operations
	mux.Handle("GET /admin", requireAuth(http.HandlerFunc(dashHandler.Dashboard)))
	mux.Handle("GET /admin/", requireAuth(http.HandlerFunc(dashHandler.Dashboard)))

	// Games
	mux.Handle("GET /admin/games", requireAuth(http.HandlerFunc(gameHandler.GameList)))
	mux.Handle("GET /admin/games/new", requireAuth(http.HandlerFunc(gameHandler.GameForm)))
	mux.Handle("POST /admin/games", requireAuth(http.HandlerFunc(gameHandler.GameCreate)))
	mux.Handle("GET /admin/games/{id}/edit", requireAuth(http.HandlerFunc(gameHandler.GameForm)))
	mux.Handle("POST /admin/games/{id}", requireAuth(http.HandlerFunc(gameHandler.GameUpdate)))
	mux.Handle("POST /admin/games/{id}/toggle", requireAuth(http.HandlerFunc(gameHandler.GameToggle)))
	mux.Handle("POST /admin/games/{id}/delete", requireAuth(http.HandlerFunc(gameHandler.GameDelete)))
	mux.Handle("POST /admin/games/{id}/reorder", requireAuth(http.HandlerFunc(gameHandler.GameReorder)))

	// Products
	mux.Handle("GET /admin/games/{id}/products", requireAuth(http.HandlerFunc(prodHandler.ProductList)))
	mux.Handle("GET /admin/games/{id}/products/import", requireAuth(http.HandlerFunc(syncHandler.ProductImport)))
	mux.Handle("POST /admin/games/{id}/products/import", requireAuth(http.HandlerFunc(syncHandler.ProductImportExec)))
	mux.Handle("GET /admin/products/{id}/edit", requireAuth(http.HandlerFunc(prodHandler.ProductForm)))
	mux.Handle("POST /admin/products/{id}", requireAuth(http.HandlerFunc(prodHandler.ProductUpdate)))
	mux.Handle("POST /admin/products/{id}/toggle", requireAuth(http.HandlerFunc(prodHandler.ProductToggle)))
	mux.Handle("POST /admin/products/{id}/delete", requireAuth(http.HandlerFunc(prodHandler.ProductDelete)))
	mux.Handle("POST /admin/products/bulk-toggle", requireAuth(http.HandlerFunc(prodHandler.ProductBulkToggle)))
	mux.Handle("POST /admin/products/bulk-margin", requireAuth(http.HandlerFunc(prodHandler.ProductBulkMargin)))

	// Orders Admin
	mux.Handle("GET /admin/orders", requireAuth(http.HandlerFunc(orderAdminHandler.OrderList)))
	mux.Handle("GET /admin/orders/{id}", requireAuth(http.HandlerFunc(orderAdminHandler.OrderDetail)))
	mux.Handle("POST /admin/orders/{id}/retry", requireAuth(http.HandlerFunc(orderAdminHandler.OrderRetry)))
	mux.Handle("POST /admin/orders/{id}/refund", requireAuth(http.HandlerFunc(orderAdminHandler.OrderRefund)))
	mux.Handle("POST /admin/orders/{id}/note", requireAuth(http.HandlerFunc(orderAdminHandler.OrderNote)))

	// Sync & Monitoring
	mux.Handle("POST /admin/sync/pricelist", requireAuth(http.HandlerFunc(syncHandler.SyncPriceList)))
	mux.Handle("GET /admin/sync/balance", requireAuth(http.HandlerFunc(syncHandler.CheckBalance)))
	mux.Handle("GET /admin/audit-log", requireAuth(http.HandlerFunc(auditHandler.AuditLog)))

	// Wrap root with logging, recovery, and security headers
	mainHandler := middleware.Chain(
		mux,
		middleware.Recoverer,
		middleware.Logger,
		middleware.SecurityHeaders,
	)

	// Background workers
	startBackgroundWorkers(orderSvc, adminSvc, st)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mainHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Server runner
	go func() {
		fmt.Printf("\n=======================================================\n")
		fmt.Printf("   🚀 Azeotopup Server Running on %s\n", cfg.BaseURL)
		fmt.Printf("   👤 Admin Dashboard: %s/admin\n", cfg.BaseURL)
		fmt.Printf("   🔑 Initial Login: %s / %s\n", cfg.AdminInitialUsername, cfg.AdminInitialPassword)
		fmt.Printf("=======================================================\n\n")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server ListenAndServe error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Azeotopup server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server gracefully stopped.")
}

func startBackgroundWorkers(orders *service.OrderService, admin *service.AdminService, st *store.SQLiteStore) {
	// Digiflazz processing orders auto-syncer (every 5 seconds)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if count, err := orders.SyncProcessingOrders(); err == nil && count > 0 {
				log.Printf("[Background Job] Auto-synced %d processing orders from Digiflazz", count)
			}
		}
	}()

	// Expired order checker (every 1 minute)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if count, err := orders.CheckExpiredOrders(); err == nil && count > 0 {
				log.Printf("[Background Job] Expired %d unpaid orders", count)
			}
		}
	}()

	// Price list auto-sync (every 6 hours)
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if count, err := admin.SyncPriceList(0); err == nil {
				log.Printf("[Background Job] Auto-synced %d products from Digiflazz", count)
			}
		}
	}()

	// Clean expired admin sessions (every 1 hour)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			_ = st.CleanExpiredSessions()
		}
	}()
}
