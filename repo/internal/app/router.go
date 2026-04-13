package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eduexchange/eduexchange/internal/audit"
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

// AppDirs holds the file-system paths used by various services.
type AppDirs struct {
	Uploads    string
	Imports    string
	Exports    string
	Reports    string
	Statements string
}

// NewRouter constructs the application router and a (not-yet-started) scheduler.
// Call scheduler.Start() to activate background jobs; tests skip this.
// loc sets the timezone used for cron scheduling (nil → UTC).
func NewRouter(pool *pgxpool.Pool, encryptionKey []byte, dirs AppDirs, loc *time.Location) (*gin.Engine, *appcron.Scheduler) {
	r := gin.New()
	r.Use(gin.Recovery())

	auditSvc := audit.NewService(pool)

	// Auth
	repo := authrepo.New(pool)
	authSvc := authservice.NewAuthService(repo, encryptionKey)
	userSvc := authservice.NewUserService(repo, auditSvc)
	authH := authhandler.New(authSvc)
	adminH := adminhandler.NewUserHandler(userSvc)

	// Catalog
	catRepo := catalogrepo.New(pool)
	catSvc := catalogservice.NewCatalogService(catRepo, auditSvc, dirs.Uploads)
	categorySvc := catalogservice.NewCategoryService(catRepo, auditSvc)
	tagSvc := catalogservice.NewTagService(catRepo, auditSvc)
	importSvc := catalogservice.NewBulkImportService(catRepo, auditSvc, dirs.Imports)
	exportSvc := catalogservice.NewBulkExportService(catRepo, auditSvc, dirs.Exports)
	catH := cataloghandler.New(catSvc, categorySvc, tagSvc, importSvc, exportSvc)

	// Gamification + Engagement
	gamRepo := gamificationrepo.New(pool)
	gamSvc := gamificationservice.NewPointsService(gamRepo, auditSvc)
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
	recSvc := recommendations.NewRecommendationService(gamRepo, strats, auditSvc)
	srchH := searchhandler.New(srchSvc, rankSvc, recSvc)

	// Wire search index updates into catalog service (Issue 6 fix)
	catSvc.SetSearchIndexUpdater(srchSvc)

	// Supplier
	supRepo := supplierrepo.New(pool)
	supSvc := supplierservice.NewSupplierService(supRepo, auditSvc, encryptionKey)
	kpiSvc := supplierservice.NewKPIService(supRepo, auditSvc)
	supH := supplierhandler.New(supSvc, kpiSvc, supRepo)

	// Messaging + SSE
	sseHub := sse.NewHub()
	msgRepo := messagingrepo.New(pool)
	notifSvc := messagingservice.NewNotificationService(msgRepo, sseHub)
	retrySvc := messagingservice.NewRetryService(msgRepo)
	msgH := messaginghandler.New(notifSvc, sseHub)

	// Wire notifications
	gamSvc.SetNotificationSender(notifSvc)
	catSvc.SetNotificationSender(notifSvc, engRepo)

	// Moderation
	modRepo := moderationrepo.New(pool)
	modSvc := moderationservice.New(modRepo, catRepo, engRepo, gamSvc, auditSvc)
	modSvc.SetNotificationSender(notifSvc)
	modH := moderationhandler.New(modSvc)

	// Supplier admin notification (admin lookup)
	supSvc.SetNotificationSender(notifSvc, func(ctx context.Context) []uuid.UUID {
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
	analyticsSvc := analyticsservice.NewAnalyticsService(analyticsRepo, auditSvc, dirs.Reports)
	analyticsH := analyticshandler.New(analyticsSvc, auditSvc)

	// Scheduler (caller decides whether to start it)
	scheduler := appcron.New(rankSvc, engRepo, pool, kpiSvc, supRepo, retrySvc, analyticsSvc, loc)

	// ── Routes ────────────────────────────────────────────────────────────────

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public
	r.GET("/search/suggest", srchH.GetSuggest)
	r.GET("/login", authH.GetLogin)
	r.GET("/register", authH.GetRegister)
	r.POST("/login", authH.PostLogin)
	r.POST("/register", authH.PostRegister)
	r.POST("/logout", authH.PostLogout)

	// Protected (auth + ban check + idempotency)
	protected := r.Group("")
	protected.Use(middleware.AuthMiddleware(pool))
	protected.Use(middleware.BanCheckMiddleware(pool))
	protected.Use(middleware.IdempotencyMiddleware(pool))
	{
		protected.GET("/", srchH.GetHome)

		// Catalog — read-only (any authenticated user)
		protected.GET("/resources", catH.GetResourceList)
		protected.GET("/resources/:id", catH.GetResourceDetail)
		protected.GET("/resources/:id/files/:fileID", catH.GetDownloadFile)
		protected.GET("/tags", catH.GetTags)

		// Catalog — authoring mutations (AUTHOR or ADMIN only)
		authorGroup := protected.Group("")
		authorGroup.Use(middleware.RequireRole("AUTHOR", "ADMIN"))
		{
			authorGroup.GET("/resources/new", catH.GetNewResource)
			authorGroup.POST("/resources", catH.PostCreateResource)
			authorGroup.GET("/resources/:id/edit", catH.GetEditResource)
			authorGroup.PUT("/resources/:id", catH.PutUpdateResource)
			authorGroup.DELETE("/resources/:id", catH.DeleteResource)
			authorGroup.POST("/resources/:id/submit", catH.PostSubmitForReview)
			authorGroup.POST("/resources/:id/revise", catH.PostRevise)
			authorGroup.POST("/resources/:id/files", catH.PostUploadFile)
			authorGroup.DELETE("/resources/:id/files/:fileID", catH.DeleteFile)
			authorGroup.POST("/tags", catH.PostCreateTag)
		}

		// Reviewer + Admin
		reviewerGroup := protected.Group("")
		reviewerGroup.Use(middleware.RequireRole("REVIEWER", "ADMIN"))
		{
			reviewerGroup.GET("/review-queue", catH.GetReviewQueue)
			reviewerGroup.POST("/resources/:id/approve", catH.PostApprove)
			reviewerGroup.POST("/resources/:id/reject", catH.PostReject)
		}

		// Admin catalog + gamification admin
		adminGroup := protected.Group("")
		adminGroup.Use(middleware.RequireRole("ADMIN"))
		{
			adminGroup.POST("/resources/:id/publish", catH.PostPublish)
			adminGroup.POST("/resources/:id/takedown", catH.PostTakedown)
			adminGroup.POST("/resources/:id/restore", catH.PostRestore)
			adminGroup.DELETE("/tags/:id", catH.DeleteTag)
			adminGroup.GET("/categories", catH.GetCategories)
			adminGroup.POST("/categories", catH.PostCreateCategory)
			adminGroup.PUT("/categories/:id", catH.PutUpdateCategory)
			adminGroup.DELETE("/categories/:id", catH.DeleteCategory)
			adminGroup.GET("/import", catH.GetImportWizard)
			adminGroup.POST("/import/upload", catH.PostImportUpload)
			adminGroup.GET("/import/:jobID/preview", catH.GetImportPreview)
			adminGroup.POST("/import/:jobID/confirm", catH.PostImportConfirm)
			adminGroup.GET("/import/:jobID/done", catH.GetImportDone)
			adminGroup.GET("/export", catH.GetExportPage)
			adminGroup.POST("/export/generate", catH.PostGenerateExport)
			adminGroup.GET("/recommendation-strategies", srchH.GetStrategyConfigs)
			adminGroup.PUT("/recommendation-strategies/:id", srchH.PutStrategyConfig)
			adminGroup.GET("/point-rules", gamH.GetPointRules)
			adminGroup.PUT("/point-rules/:id", gamH.PutPointRule)
			// Audit logs
			adminGroup.GET("/audit-logs", analyticsH.GetAuditLogs)
			adminGroup.POST("/audit-logs/export", analyticsH.PostExportAuditLog)
		}

		// Engagement
		protected.POST("/resources/:id/vote", engH.PostVote)
		protected.DELETE("/resources/:id/vote", engH.DeleteVote)
		protected.POST("/resources/:id/favorite", engH.PostFavorite)
		protected.GET("/favorites", engH.GetFavorites)
		protected.POST("/follows", engH.PostFollow)

		// Gamification
		protected.GET("/users/:id/points", gamH.GetUserPoints)
		protected.GET("/users/:id/badges", gamH.GetUserBadges)
		protected.GET("/leaderboard", gamH.GetLeaderboard)

		// Search
		protected.GET("/search", srchH.GetSearch)
		protected.GET("/search/history", srchH.GetHistory)
		protected.DELETE("/search/history", srchH.DeleteHistory)
		protected.GET("/rankings/bestsellers", srchH.GetBestsellers)
		protected.GET("/rankings/new-releases", srchH.GetNewReleases)
		protected.GET("/recommendations", srchH.GetRecommendations)

		// Reports (any authenticated user)
		protected.POST("/reports", modH.CreateReport)

		// Moderation (Reviewer + Admin)
		moderationGroup := protected.Group("/moderation")
		moderationGroup.Use(middleware.RequireRole("REVIEWER", "ADMIN"))
		{
			moderationGroup.GET("/reports", modH.ListReports)
			moderationGroup.GET("/reports/:id", modH.GetReportDetail)
			moderationGroup.POST("/reports/:id/assign", modH.AssignReport)
			moderationGroup.POST("/reports/:id/resolve", modH.ResolveReport)
			moderationGroup.POST("/reports/:id/dismiss", modH.DismissReport)
			moderationGroup.POST("/resources/:id/takedown", modH.TakedownResource)
			moderationGroup.GET("/anomalies", modH.ListAnomalyFlags)
			moderationGroup.POST("/anomalies/:id/review", modH.ReviewAnomaly)
		}

		// Moderation (Admin only)
		adminModerationGroup := protected.Group("/moderation")
		adminModerationGroup.Use(middleware.RequireRole("ADMIN"))
		{
			adminModerationGroup.POST("/resources/:id/restore", modH.RestoreResource)
			adminModerationGroup.POST("/users/:id/ban", modH.BanUser)
			adminModerationGroup.POST("/users/:id/unban", modH.UnbanUser)
		}

		// Supplier (Supplier + Admin)
		supplierGroup := protected.Group("/supplier")
		supplierGroup.Use(middleware.RequireRole("SUPPLIER", "ADMIN"))
		{
			supplierGroup.GET("/portal", supH.GetPortal)
			supplierGroup.GET("/orders", supH.GetOrderList)
			supplierGroup.GET("/orders/new", supH.GetOrderForm)
			supplierGroup.GET("/orders/:id", supH.GetOrderDetail)
			supplierGroup.PUT("/orders/:id/confirm", supH.PutConfirmDeliveryDate)
			supplierGroup.POST("/orders/:id/confirm", supH.PutConfirmDeliveryDate)
			supplierGroup.POST("/orders/:id/asn", supH.PostSubmitASN)
		}

		// Supplier Admin
		supplierAdminGroup := protected.Group("")
		supplierAdminGroup.Use(middleware.RequireRole("ADMIN"))
		{
			supplierAdminGroup.GET("/suppliers", supH.GetSupplierList)
			supplierAdminGroup.POST("/suppliers", supH.PostCreateSupplier)
			supplierAdminGroup.GET("/suppliers/:id", supH.GetSupplierDetail)
			supplierAdminGroup.GET("/suppliers/:id/kpis", supH.GetKPIDashboard)
			supplierAdminGroup.POST("/suppliers/:id/kpis/recalculate", supH.PostRecalculateKPIs)
			supplierAdminGroup.POST("/supplier/orders", supH.PostCreateOrder)
			supplierAdminGroup.POST("/supplier/orders/:id/receive", supH.PostConfirmReceipt)
			supplierAdminGroup.POST("/supplier/orders/:id/qc", supH.PostSubmitQCResult)
			supplierAdminGroup.POST("/supplier/orders/:id/close", supH.PostCloseOrder)
			supplierAdminGroup.POST("/supplier/orders/:id/cancel", supH.PostCancelOrder)
		}

		// Messaging
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
		protected.GET("/events/stream", msgH.GetEventStream)

		// Analytics
		protected.GET("/analytics/dashboard", analyticsH.GetDashboard)

		analyticsAdminGroup := protected.Group("/analytics")
		analyticsAdminGroup.Use(middleware.RequireRole("ADMIN"))
		{
			analyticsAdminGroup.GET("/reports", analyticsH.GetReportList)
			analyticsAdminGroup.POST("/reports/generate", analyticsH.PostGenerateReport)
			analyticsAdminGroup.GET("/reports/:id/download", analyticsH.GetReportDownload)
		}
	}

	// Admin user management pages
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

	return r, scheduler
}
