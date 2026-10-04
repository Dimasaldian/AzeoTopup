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
	userSvc := service.NewUserService(st)

	apiGamesClient := service.NewApiGamesClient(cfg.ApiGamesMerchantID, cfg.ApiGamesSecretKey)
	if apiGamesClient.IsConfigured() {
		log.Printf("[ApiGames] CONNECTED! Merchant ID: %s (LIVE Nickname Check mode enabled).", apiGamesClient.MerchantID)
	} else {
		log.Printf("[ApiGames] Credentials not set in .env. Running in SIMULATION mode.")
	}

	// Public Handlers
	pageHandler := handler.NewPageHandler(st, orderSvc, !agpClient.IsConfigured())
	orderHandler := handler.NewOrderHandler(st, orderSvc, apiGamesClient)
	userHandler := handler.NewUserHandler(st, userSvc, 86400*30)
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
	voucherAdminHandler := admin.NewVoucherAdminHandler(st, userSvc)
	userAdminHandler := admin.NewUserAdminHandler(st)
	adminUserHandler := admin.NewAdminUserHandler(st)
	ticketHandler := handler.NewTicketHandler(st)
	ticketAdminHandler := admin.NewTicketAdminHandler(st)

	// Middleware
	requireAuth := middleware.RequireAdmin(st)
	requireSuperAdmin := middleware.RequireSuperAdmin(st)
	requireUser := middleware.RequireUser

	// Routing with standard Go 1.22+ ServeMux
	mux := http.NewServeMux()

	// Static & Upload files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	// Public Pages
	mux.HandleFunc("GET /", pageHandler.PageHome)
	mux.HandleFunc("GET /game/{code}", pageHandler.PageGame)
	mux.HandleFunc("GET /order/{id}", pageHandler.PageOrderStatus)
	mux.HandleFunc("GET /bantuan", ticketHandler.PageSupport)
	mux.HandleFunc("POST /bantuan", ticketHandler.SubmitTicket)
	mux.HandleFunc("GET /bantuan/{id}", ticketHandler.PageTicketDetail)

	// Customer Auth & Account
	mux.HandleFunc("GET /login", userHandler.LoginPage)
	mux.HandleFunc("POST /login", userHandler.LoginSubmit)
	mux.HandleFunc("GET /register", userHandler.RegisterPage)
	mux.HandleFunc("POST /register", userHandler.RegisterSubmit)
	mux.HandleFunc("POST /logout", userHandler.Logout)
	mux.Handle("GET /account", requireUser(http.HandlerFunc(userHandler.AccountPage)))
	mux.Handle("POST /account/redeem", requireUser(http.HandlerFunc(userHandler.RedeemVoucherSubmit)))
	mux.Handle("POST /api/account/redeem", requireUser(http.HandlerFunc(userHandler.APIRedeemVoucher)))

	// Public APIs
	mux.HandleFunc("POST /api/order", orderHandler.CreateOrder)
	mux.HandleFunc("GET /api/order/{id}/status", orderHandler.GetOrderStatus)
	mux.HandleFunc("POST /api/order/{id}/simulate-pay", orderHandler.SimulatePayment)
	mux.HandleFunc("GET /api/check-username", orderHandler.CheckUsername)

	// Webhooks
	mux.Handle("POST /webhook/autogopay", webhookAGP)
	mux.Handle("POST /webhook/digiflazz", webhookDF)

	// Admin Auth
	mux.HandleFunc("GET /admin/login", authHandler.LoginPage)
	mux.HandleFunc("POST /admin/login", authHandler.LoginSubmit)
	mux.HandleFunc("POST /admin/logout", authHandler.Logout)

	// Admin Dashboard & Protected Operations (All Admins)
	mux.Handle("GET /admin", requireAuth(http.HandlerFunc(dashHandler.Dashboard)))
	mux.Handle("GET /admin/", requireAuth(http.HandlerFunc(dashHandler.Dashboard)))

	// Games & Products (All Admins)
	mux.Handle("GET /admin/games", requireAuth(http.HandlerFunc(gameHandler.GameList)))
	mux.Handle("GET /admin/games/new", requireAuth(http.HandlerFunc(gameHandler.GameForm)))
	mux.Handle("POST /admin/games", requireAuth(http.HandlerFunc(gameHandler.GameCreate)))
	mux.Handle("GET /admin/games/{id}/edit", requireAuth(http.HandlerFunc(gameHandler.GameForm)))
	mux.Handle("POST /admin/games/{id}", requireAuth(http.HandlerFunc(gameHandler.GameUpdate)))
	mux.Handle("POST /admin/games/{id}/toggle", requireAuth(http.HandlerFunc(gameHandler.GameToggle)))
	mux.Handle("POST /admin/games/{id}/delete", requireSuperAdmin(http.HandlerFunc(gameHandler.GameDelete)))
	mux.Handle("POST /admin/games/{id}/reorder", requireAuth(http.HandlerFunc(gameHandler.GameReorder)))

	// Products
	mux.Handle("GET /admin/games/{id}/products", requireAuth(http.HandlerFunc(prodHandler.ProductList)))
	mux.Handle("GET /admin/games/{id}/products/import", requireSuperAdmin(http.HandlerFunc(syncHandler.ProductImport)))
	mux.Handle("POST /admin/games/{id}/products/import", requireSuperAdmin(http.HandlerFunc(syncHandler.ProductImportExec)))
	mux.Handle("GET /admin/products/{id}/edit", requireAuth(http.HandlerFunc(prodHandler.ProductForm)))
	mux.Handle("POST /admin/products/{id}", requireAuth(http.HandlerFunc(prodHandler.ProductUpdate)))
	mux.Handle("POST /admin/products/{id}/toggle", requireAuth(http.HandlerFunc(prodHandler.ProductToggle)))
	mux.Handle("POST /admin/products/{id}/delete", requireSuperAdmin(http.HandlerFunc(prodHandler.ProductDelete)))
	mux.Handle("POST /admin/products/bulk-toggle", requireAuth(http.HandlerFunc(prodHandler.ProductBulkToggle)))
	mux.Handle("POST /admin/products/bulk-margin", requireSuperAdmin(http.HandlerFunc(prodHandler.ProductBulkMargin)))

	// Orders Admin (All Admins)
	mux.Handle("GET /admin/orders", requireAuth(http.HandlerFunc(orderAdminHandler.OrderList)))
	mux.Handle("GET /admin/orders/{id}", requireAuth(http.HandlerFunc(orderAdminHandler.OrderDetail)))
	mux.Handle("POST /admin/orders/{id}/retry", requireAuth(http.HandlerFunc(orderAdminHandler.OrderRetry)))
	mux.Handle("POST /admin/orders/{id}/refund", requireAuth(http.HandlerFunc(orderAdminHandler.OrderRefund)))
	mux.Handle("POST /admin/orders/{id}/note", requireAuth(http.HandlerFunc(orderAdminHandler.OrderNote)))

	// Users Admin (All Admins)
	mux.Handle("GET /admin/users", requireAuth(http.HandlerFunc(userAdminHandler.UserList)))

	// Customer Support / Tickets Admin (All Admins)
	mux.Handle("GET /admin/tickets", requireAuth(http.HandlerFunc(ticketAdminHandler.TicketList)))
	mux.Handle("GET /admin/tickets/{id}", requireAuth(http.HandlerFunc(ticketAdminHandler.TicketDetail)))
	mux.Handle("POST /admin/tickets/{id}/update", requireAuth(http.HandlerFunc(ticketAdminHandler.TicketUpdate)))

	// Vouchers & AZcoin (Superadmin Only)
	mux.Handle("GET /admin/vouchers", requireSuperAdmin(http.HandlerFunc(voucherAdminHandler.VoucherList)))
	mux.Handle("POST /admin/vouchers/generate", requireSuperAdmin(http.HandlerFunc(voucherAdminHandler.VoucherGenerate)))
	mux.Handle("POST /admin/vouchers/{id}/delete", requireSuperAdmin(http.HandlerFunc(voucherAdminHandler.VoucherDelete)))
	mux.Handle("POST /admin/vouchers/discount", requireSuperAdmin(http.HandlerFunc(voucherAdminHandler.UpdateDiscount)))

	// Admin Account Management (Superadmin Only)
	mux.Handle("GET /admin/admins", requireSuperAdmin(http.HandlerFunc(adminUserHandler.List)))
	mux.Handle("POST /admin/admins", requireSuperAdmin(http.HandlerFunc(adminUserHandler.Create)))
	mux.Handle("POST /admin/admins/{id}/toggle", requireSuperAdmin(http.HandlerFunc(adminUserHandler.Toggle)))
	mux.Handle("POST /admin/admins/{id}/edit", requireSuperAdmin(http.HandlerFunc(adminUserHandler.Update)))
	mux.Handle("POST /admin/admins/{id}/delete", requireSuperAdmin(http.HandlerFunc(adminUserHandler.Delete)))

	// Sync & Monitoring (Superadmin Only)
	mux.Handle("POST /admin/sync/pricelist", requireSuperAdmin(http.HandlerFunc(syncHandler.SyncPriceList)))
	mux.Handle("GET /admin/sync/balance", requireSuperAdmin(http.HandlerFunc(syncHandler.CheckBalance)))
	mux.Handle("GET /admin/audit-log", requireSuperAdmin(http.HandlerFunc(auditHandler.AuditLog)))

	// Wrap root with logging, recovery, user session auth, and security headers
	mainHandler := middleware.Chain(
		mux,
		middleware.Recoverer,
		middleware.Logger,
		middleware.SecurityHeaders,
		middleware.UserAuth(st),
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
			_ = st.CleanExpiredUserSessions()
		}
	}()
}
