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

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/config"
	appcron "github.com/eduexchange/eduexchange/internal/cron"
	adminhandler "github.com/eduexchange/eduexchange/internal/handler/admin"
	analyticshandler "github.com/eduexchange/eduexchange/internal/handler/analytics"
	authhandler "github.com/eduexchange/eduexchange/internal/handler/auth"
	cataloghandler "github.com/eduexchange/eduexchange/internal/handler/catalog"
	engagementhandler "github.com/eduexchange/eduexchange/internal/handler/engagement"
	gamificationhandler "github.com/eduexchange/eduexchange/internal/handler/gamification"
	messaginghandler "github.com/eduexchange/eduexchange/internal/handler/messaging"
	moderationhandler "github.com/eduexchange/eduexchange/internal/handler/moderation"
	searchhandler "github.com/eduexchange/eduexchange/internal/handler/search"
	supplierhandler "github.com/eduexchange/eduexchange/internal/handler/supplier"
	"github.com/eduexchange/eduexchange/internal/middleware"
	analyticsrepo "github.com/eduexchange/eduexchange/internal/repository/analytics"
	authrepo "github.com/eduexchange/eduexchange/internal/repository/auth"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	messagingrepo "github.com/eduexchange/eduexchange/internal/repository/messaging"
	moderationrepo "github.com/eduexchange/eduexchange/internal/repository/moderation"
	searchrepo "github.com/eduexchange/eduexchange/internal/repository/search"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	analyticsservice "github.com/eduexchange/eduexchange/internal/service/analytics"
	authservice "github.com/eduexchange/eduexchange/internal/service/auth"
	catalogservice "github.com/eduexchange/eduexchange/internal/service/catalog"
	engagementservice "github.com/eduexchange/eduexchange/internal/service/engagement"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
	messagingservice "github.com/eduexchange/eduexchange/internal/service/messaging"
	moderationservice "github.com/eduexchange/eduexchange/internal/service/moderation"
	"github.com/eduexchange/eduexchange/internal/service/recommendations"
	searchservice "github.com/eduexchange/eduexchange/internal/service/search"
	supplierservice "github.com/eduexchange/eduexchange/internal/service/supplier"
	"github.com/eduexchange/eduexchange/internal/sse"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			runMigrate()
			return
		case "seed":
			runSeed()
			return
		case "migrate-fresh":
			runMigrateFresh()
			if len(os.Args) > 2 && os.Args[2] == "--seed" {
				runSeed()
			}
			return
		case "serve":
			// fall through to startup sequence
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()

	// Wait for database to be ready (replaces shell-level wait loop)
	log.Println("Waiting for database...")
	pool, err := waitForDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database never became ready: %v", err)
	}
	defer pool.Close()
	log.Println("Database is ready.")

	// Run migrations
	log.Println("Running migrations...")
	runMigrate()

	// Seed if empty
	var userCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err == nil && userCount == 0 {
		log.Println("Seeding database...")
		runSeed()
	}

	sseHub := sse.NewHub()
	auditSvc := audit.NewService(pool)

	r := setupRouter(pool, sseHub, auditSvc, cfg)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}

	go func() {
		log.Printf("EduExchange starting on port %d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	log.Println("Server stopped")
}

func waitForDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	for i := 0; i < 30; i++ {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err == nil {
			if err := pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		log.Printf("  DB not ready (attempt %d/30), retrying in 2s...", i+1)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("database did not become ready after 60 seconds")
}

func setupRouter(pool *pgxpool.Pool, sseHub *sse.Hub, auditSvc *audit.Service, cfg *config.Config) *gin.Engine {
	r := gin.Default()

	// Static files
	r.Static("/static", "./static")

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Wire dependencies
	repo := authrepo.New(pool)
	authSvc := authservice.NewAuthService(repo)
	userSvc := authservice.NewUserService(repo, auditSvc)
	authH := authhandler.New(authSvc)
	adminH := adminhandler.NewUserHandler(userSvc)

	catRepo := catalogrepo.New(pool)
	catSvc := catalogservice.NewCatalogService(catRepo, auditSvc, "data/uploads")
	categorySvc := catalogservice.NewCategoryService(catRepo, auditSvc)
	tagSvc := catalogservice.NewTagService(catRepo, auditSvc)
	importSvc := catalogservice.NewBulkImportService(catRepo, auditSvc, "data/imports")
	exportSvc := catalogservice.NewBulkExportService(catRepo, auditSvc, "data/exports")
	catH := cataloghandler.New(catSvc, categorySvc, tagSvc, importSvc, exportSvc)

	// Engagement + Gamification
	gamRepo := gamificationrepo.New(pool)
	gamSvc := gamificationservice.NewPointsService(gamRepo)
	rankSvc := gamificationservice.NewRankingService(gamRepo)

	engRepo := engagementrepo.New(pool)
	engSvc := engagementservice.NewEngagementService(engRepo, catRepo, auditSvc, gamSvc)
	engH := engagementhandler.New(engSvc)
	gamH := gamificationhandler.New(gamSvc)

	// Search + Recommendations
	srchRepo := searchrepo.New(pool)
	srchSvc := searchservice.NewSearchService(srchRepo)

	strats := []recommendations.RecommendationStrategy{
		recommendations.NewMostEngagedCategories(gamRepo, pool),
		recommendations.NewFollowedAuthorNewContent(engRepo, pool),
		recommendations.NewSimilarTagAffinity(gamRepo, pool),
	}
	recSvc := recommendations.NewRecommendationService(gamRepo, strats)
	srchH := searchhandler.New(srchSvc, rankSvc, recSvc)

	// Supplier services
	supplierRepo := supplierrepo.New(pool)
	supplierSvc := supplierservice.NewSupplierService(supplierRepo, auditSvc)
	kpiSvc := supplierservice.NewKPIService(supplierRepo)
	supplierH := supplierhandler.New(supplierSvc, kpiSvc, supplierRepo)

	// Messaging (notifications)
	msgRepo := messagingrepo.New(pool)
	notifSvc := messagingservice.NewNotificationService(msgRepo, sseHub)
	retrySvc := messagingservice.NewRetryService(msgRepo)
	msgH := messaginghandler.New(notifSvc, sseHub)

	// Wire notifications into existing services
	gamSvc.SetNotificationSender(notifSvc)
	catSvc.SetNotificationSender(notifSvc, engRepo)
	modRepo := moderationrepo.New(pool)
	modSvc := moderationservice.New(modRepo, catRepo, engRepo, gamSvc, auditSvc)
	modSvc.SetNotificationSender(notifSvc)
	modH := moderationhandler.New(modSvc)
	supplierSvc.SetNotificationSender(notifSvc, func(ctx context.Context) []uuid.UUID {
		rows, err := pool.Query(ctx, `SELECT DISTINCT ur.user_id FROM user_roles ur WHERE ur.role = 'ADMIN'`)
		if err != nil {
			return nil
		}
		defer rows.Close()
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
		}
		return ids
	})

	// Analytics
	analyticsRepo := analyticsrepo.New(pool)
	analyticsSvc := analyticsservice.NewAnalyticsService(analyticsRepo, auditSvc, "data/exports/reports")
	analyticsH := analyticshandler.New(analyticsSvc, auditSvc)

	// Cron scheduler
	scheduler := appcron.New(rankSvc, engRepo, pool, kpiSvc, supplierRepo, retrySvc, analyticsSvc)
	scheduler.Start()

	// Public auth pages (browser-facing, HTML)
	r.GET("/login", authH.GetLogin)
	r.GET("/register", authH.GetRegister)
	r.POST("/login", authH.PostLogin)
	r.POST("/register", authH.PostRegister)
	r.POST("/logout", authH.PostLogout)

	// API v1 routes
	v1 := r.Group("/api/v1")

	// Public auth routes (API aliases kept for compatibility)
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/login", authH.PostLogin)
		authGroup.POST("/register", authH.PostRegister)
		authGroup.POST("/logout", authH.PostLogout)
	}

	// Protected HTML page routes (root-level, not under /api/v1)
	protected := r.Group("")
	protected.Use(middleware.AuthMiddleware(pool))
	protected.Use(middleware.BanCheckMiddleware(pool))
	protected.Use(middleware.IdempotencyMiddleware(pool))

	// ── Catalog HTML pages ────────────────────────────────────────────────────
	// Resources (all authenticated users)
	protected.GET("/resources", catH.GetResourceList)
	protected.GET("/resources/new", catH.GetNewResource)
	protected.POST("/resources", catH.PostCreateResource)
	protected.GET("/resources/:id", catH.GetResourceDetail)
	protected.GET("/resources/:id/edit", catH.GetEditResource)
	protected.PUT("/resources/:id", catH.PutUpdateResource)
	protected.DELETE("/resources/:id", catH.DeleteResource)

	// Workflow (author actions)
	protected.POST("/resources/:id/submit", catH.PostSubmitForReview)
	protected.POST("/resources/:id/revise", catH.PostRevise)

	// Files
	protected.POST("/resources/:id/files", catH.PostUploadFile)
	protected.GET("/resources/:id/files/:fileID", catH.GetDownloadFile)
	protected.DELETE("/resources/:id/files/:fileID", catH.DeleteFile)

	// Tags (Author+)
	protected.GET("/tags", catH.GetTags)
	protected.POST("/tags", catH.PostCreateTag)

	// Reviewer/Admin workflow
	reviewerGroup := protected.Group("")
	reviewerGroup.Use(middleware.RequireRole("REVIEWER", "ADMIN"))
	{
		reviewerGroup.GET("/review-queue", catH.GetReviewQueue)
		reviewerGroup.POST("/resources/:id/approve", catH.PostApprove)
		reviewerGroup.POST("/resources/:id/reject", catH.PostReject)
	}

	// Admin-only catalog actions
	catAdminGroup := protected.Group("")
	catAdminGroup.Use(middleware.RequireRole("ADMIN"))
	{
		catAdminGroup.POST("/resources/:id/publish", catH.PostPublish)
		catAdminGroup.POST("/resources/:id/takedown", catH.PostTakedown)
		catAdminGroup.POST("/resources/:id/restore", catH.PostRestore)
		catAdminGroup.DELETE("/tags/:id", catH.DeleteTag)
		catAdminGroup.GET("/categories", catH.GetCategories)
		catAdminGroup.POST("/categories", catH.PostCreateCategory)
		catAdminGroup.PUT("/categories/:id", catH.PutUpdateCategory)
		catAdminGroup.DELETE("/categories/:id", catH.DeleteCategory)
		catAdminGroup.GET("/import", catH.GetImportWizard)
		catAdminGroup.POST("/import/upload", catH.PostImportUpload)
		catAdminGroup.GET("/import/:jobID/preview", catH.GetImportPreview)
		catAdminGroup.POST("/import/:jobID/confirm", catH.PostImportConfirm)
		catAdminGroup.GET("/import/:jobID/done", catH.GetImportDone)
		catAdminGroup.GET("/export", catH.GetExportPage)
		catAdminGroup.POST("/export/generate", catH.PostGenerateExport)
		catAdminGroup.GET("/recommendation-strategies", srchH.GetStrategyConfigs)
		catAdminGroup.PUT("/recommendation-strategies/:id", srchH.PutStrategyConfig)
	}

	// Engagement routes
	protected.POST("/resources/:id/vote", engH.PostVote)
	protected.DELETE("/resources/:id/vote", engH.DeleteVote)
	protected.POST("/resources/:id/favorite", engH.PostFavorite)
	protected.GET("/favorites", engH.GetFavorites)
	protected.POST("/follows", engH.PostFollow)

	// Gamification routes
	protected.GET("/users/:id/points", gamH.GetUserPoints)
	protected.GET("/users/:id/badges", gamH.GetUserBadges)
	protected.GET("/leaderboard", gamH.GetLeaderboard)

	// Gamification admin routes
	catAdminGroup.GET("/point-rules", gamH.GetPointRules)
	catAdminGroup.PUT("/point-rules/:id", gamH.PutPointRule)

	// Search + Rankings + Recommendations
	protected.GET("/", srchH.GetHome)
	protected.GET("/search", srchH.GetSearch)
	r.GET("/search/suggest", srchH.GetSuggest) // public for type-ahead
	protected.GET("/search/history", srchH.GetHistory)
	protected.DELETE("/search/history", srchH.DeleteHistory)
	protected.GET("/rankings/bestsellers", srchH.GetBestsellers)
	protected.GET("/rankings/new-releases", srchH.GetNewReleases)
	protected.GET("/recommendations", srchH.GetRecommendations)

	// Moderation routes
	moderationGroup := protected.Group("/moderation")
	moderationGroup.Use(middleware.RequireRole("REVIEWER", "ADMIN"))
	{
		moderationGroup.GET("/reports", modH.ListReports)
		moderationGroup.POST("/reports", modH.CreateReport)
		moderationGroup.GET("/reports/:id", modH.GetReportDetail)
		moderationGroup.POST("/reports/:id/assign", modH.AssignReport)
		moderationGroup.POST("/reports/:id/resolve", modH.ResolveReport)
		moderationGroup.POST("/reports/:id/dismiss", modH.DismissReport)
		moderationGroup.POST("/resources/:id/takedown", modH.TakedownResource)
		moderationGroup.POST("/resources/:id/restore", modH.RestoreResource)
		moderationGroup.POST("/users/:id/ban", modH.BanUser)
		moderationGroup.DELETE("/users/:id/ban", modH.UnbanUser)
		moderationGroup.GET("/anomalies", modH.ListAnomalyFlags)
		moderationGroup.POST("/anomalies/:id/review", modH.ReviewAnomaly)
	}

	// Supplier routes (SUPPLIER + ADMIN)
	supplierGroup := protected.Group("/supplier")
	supplierGroup.Use(middleware.RequireRole("SUPPLIER", "ADMIN"))
	{
		supplierGroup.GET("/portal", supplierH.GetPortal)
		supplierGroup.GET("/orders", supplierH.GetOrderList)
		supplierGroup.GET("/orders/new", supplierH.GetOrderForm)
		supplierGroup.GET("/orders/:id", supplierH.GetOrderDetail)
		supplierGroup.PUT("/orders/:id/confirm", supplierH.PutConfirmDeliveryDate)
		supplierGroup.POST("/orders/:id/confirm", supplierH.PutConfirmDeliveryDate)
		supplierGroup.POST("/orders/:id/asn", supplierH.PostSubmitASN)
	}

	// Supplier Admin routes
	supplierAdminGroup := protected.Group("")
	supplierAdminGroup.Use(middleware.RequireRole("ADMIN"))
	{
		supplierAdminGroup.GET("/suppliers", supplierH.GetSupplierList)
		supplierAdminGroup.POST("/suppliers", supplierH.PostCreateSupplier)
		supplierAdminGroup.GET("/suppliers/:id", supplierH.GetSupplierDetail)
		supplierAdminGroup.GET("/suppliers/:id/kpis", supplierH.GetKPIDashboard)
		supplierAdminGroup.POST("/suppliers/:id/kpis/recalculate", supplierH.PostRecalculateKPIs)
		supplierAdminGroup.POST("/supplier/orders", supplierH.PostCreateOrder)
		supplierAdminGroup.POST("/supplier/orders/:id/receive", supplierH.PostConfirmReceipt)
		supplierAdminGroup.POST("/supplier/orders/:id/qc", supplierH.PostSubmitQCResult)
		supplierAdminGroup.POST("/supplier/orders/:id/close", supplierH.PostCloseOrder)
		supplierAdminGroup.POST("/supplier/orders/:id/cancel", supplierH.PostCancelOrder)
	}

	// Messaging routes
	messagingGroup := protected.Group("/messaging")
	{
		messagingGroup.GET("", msgH.GetCenter)
		messagingGroup.GET("/notifications", msgH.GetNotifications)
		messagingGroup.GET("/notifications/unread-count", msgH.GetUnreadCount)
		messagingGroup.POST("/notifications/:id/read", msgH.PostMarkRead)
		messagingGroup.POST("/notifications/read-all", msgH.PostMarkAllRead)
		messagingGroup.GET("/subscriptions", msgH.GetSubscriptions)
		messagingGroup.PUT("/subscriptions", msgH.PutSubscriptions)
	}
	// SSE stream (replaces old placeholder)
	protected.GET("/events/stream", msgH.GetEventStream)

	// Analytics routes
	analyticsGroup := protected.Group("/analytics")
	{
		analyticsGroup.GET("/dashboard", analyticsH.GetDashboard)
	}
	analyticsAdminGroup := protected.Group("/analytics")
	analyticsAdminGroup.Use(middleware.RequireRole("ADMIN"))
	{
		analyticsAdminGroup.GET("/reports", analyticsH.GetReportList)
		analyticsAdminGroup.POST("/reports/generate", analyticsH.PostGenerateReport)
		analyticsAdminGroup.GET("/reports/:id/download", analyticsH.GetReportDownload)
	}

	// Admin user management (HTML pages)
	adminPages := r.Group("/admin")
	adminPages.Use(middleware.AuthMiddleware(pool))
	adminPages.Use(middleware.RequireRole("ADMIN"))
	{
		adminPages.GET("/users", adminH.GetUserList)
		adminPages.GET("/users/:id", adminH.GetUserDetail)
		adminPages.POST("/users/:id/status", adminH.PostTransitionStatus)
		adminPages.POST("/users/:id/roles/assign", adminH.PostAssignRole)
		adminPages.POST("/users/:id/roles/remove", adminH.PostRemoveRole)
		adminPages.POST("/users/:id/unlock", adminH.PostUnlockUser)
	}

	// Admin API routes
	adminGroup := protected.Group("/admin")
	adminGroup.Use(middleware.RequireRole("ADMIN"))
	{
		adminGroup.GET("/audit-logs", analyticsH.GetAuditLogs)
		adminGroup.POST("/audit-logs/export", analyticsH.PostExportAuditLog)
	}

	_ = cfg // used in future steps

	return r
}

func runMigrate() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		log.Fatalf("Failed to create migrator: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migrations applied successfully")
}

func runMigrateFresh() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		log.Fatalf("Failed to create migrator: %v", err)
	}

	if err := m.Drop(); err != nil {
		log.Fatalf("Drop failed: %v", err)
	}

	m2, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		log.Fatalf("Failed to create migrator: %v", err)
	}

	if err := m2.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Database reset and migrations applied successfully")
}

func runSeed() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	adminPassword, _ := bcrypt.GenerateFromPassword([]byte("Admin12345!@"), bcrypt.DefaultCost)
	authorPassword, _ := bcrypt.GenerateFromPassword([]byte("Author12345!@"), bcrypt.DefaultCost)
	reviewerPassword, _ := bcrypt.GenerateFromPassword([]byte("Review12345!@"), bcrypt.DefaultCost)
	supplierPassword, _ := bcrypt.GenerateFromPassword([]byte("Supply12345!@"), bcrypt.DefaultCost)
	userPassword, _ := bcrypt.GenerateFromPassword([]byte("Teach12345!@"), bcrypt.DefaultCost)

	type seedUser struct {
		id       uuid.UUID
		username string
		email    string
		password []byte
		roles    []string
		display  string
	}

	users := []seedUser{
		{uuid.New(), "admin", "admin@eduexchange.local", adminPassword, []string{"ADMIN"}, "System Administrator"},
		{uuid.New(), "author1", "author1@eduexchange.local", authorPassword, []string{"AUTHOR"}, "Demo Author"},
		{uuid.New(), "reviewer1", "reviewer1@eduexchange.local", reviewerPassword, []string{"REVIEWER"}, "Demo Reviewer"},
		{uuid.New(), "supplier1", "supplier1@eduexchange.local", supplierPassword, []string{"SUPPLIER"}, "Demo Supplier"},
		{uuid.New(), "teacher1", "teacher1@eduexchange.local", userPassword, []string{}, "Demo Teacher"},
	}

	for _, u := range users {
		// Upsert: always reset seeded account password, status, and lockout.
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, username, email, password_hash, status, failed_login_count, locked_until, version)
			 VALUES ($1, $2, $3, $4, 'ACTIVE', 0, NULL, 1)
			 ON CONFLICT (username) DO UPDATE SET
			   password_hash = EXCLUDED.password_hash,
			   status = 'ACTIVE',
			   failed_login_count = 0,
			   locked_until = NULL,
			   updated_at = NOW()`,
			u.id, u.username, u.email, string(u.password),
		)
		if err != nil {
			log.Printf("Warning: failed to upsert user %s: %v", u.username, err)
			continue
		}

		// Get actual user ID (handles the ON CONFLICT case where a different ID was used)
		var actualID uuid.UUID
		err = pool.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, u.username).Scan(&actualID)
		if err != nil {
			continue
		}

		for _, role := range u.roles {
			pool.Exec(ctx,
				`INSERT INTO user_roles (user_id, role) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				actualID, role,
			)
		}

		pool.Exec(ctx,
			`INSERT INTO user_profiles (user_id, display_name) VALUES ($1, $2)
			 ON CONFLICT (user_id) DO UPDATE SET display_name = EXCLUDED.display_name`,
			actualID, u.display,
		)
	}

	// Seed a supplier record linked to supplier1 if not already linked.
	var supplier1ID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE username = 'supplier1'`).Scan(&supplier1ID); err == nil {
		var existingCount int
		pool.QueryRow(ctx, `SELECT COUNT(*) FROM suppliers WHERE user_id = $1`, supplier1ID).Scan(&existingCount)
		if existingCount == 0 {
			pool.Exec(ctx,
				`INSERT INTO suppliers (id, name, contact_json, contact_mask, tier, status, user_id, version)
				 VALUES (uuid_generate_v4(), 'Demo Supplier Co.', '{"email":"supplier1@eduexchange.local","phone":"555-0100"}', 'supplier1@...', 'SILVER', 'ACTIVE', $1, 1)`,
				supplier1ID,
			)
		}
	}

	log.Println("Seed data created successfully")
}
