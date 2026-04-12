package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/middleware"
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

var (
	testPool   *pgxpool.Pool
	testServer *httptest.Server
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	dbURL := os.Getenv("DATABASE_URL_TEST")
	if dbURL == "" {
		dbURL = "postgres://eduexchange:eduexchange@db:5432/eduexchange_test?sslmode=disable"
	}

	ctx := context.Background()

	// Connect with retry
	var err error
	for i := 0; i < 20; i++ {
		testPool, err = pgxpool.New(ctx, dbURL)
		if err == nil {
			if err = testPool.Ping(ctx); err == nil {
				break
			}
			testPool.Close()
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		panic("failed to connect to test DB: " + err.Error())
	}
	defer testPool.Close()

	// Run migrations
	m2, err := migrate.New("file://../../migrations", dbURL)
	if err != nil {
		panic("failed to create migrator: " + err.Error())
	}
	// If a previous run left a dirty version, force-roll it back so Up() can retry.
	if v, dirty, verr := m2.Version(); verr == nil && dirty && v > 0 {
		if ferr := m2.Force(int(v) - 1); ferr != nil {
			panic("failed to force migration version: " + ferr.Error())
		}
	}
	if err := m2.Up(); err != nil && err != migrate.ErrNoChange {
		panic("migration failed: " + err.Error())
	}

	// Truncate before each suite (done in TestMain so all tests start clean)
	truncateAll(ctx)

	// Build test server
	testServer = httptest.NewServer(buildRouter())

	code := m.Run()
	testServer.Close()
	os.Exit(code)
}

func buildRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	repo := authrepo.New(testPool)
	auditSvc := audit.NewService(testPool)
	authSvc := authservice.NewAuthService(repo)
	userSvc := authservice.NewUserService(repo, auditSvc)

	catRepo := catalogrepo.New(testPool)
	catSvc := catalogservice.NewCatalogService(catRepo, auditSvc, "data/uploads")
	categorySvc := catalogservice.NewCategoryService(catRepo, auditSvc)
	tagSvc := catalogservice.NewTagService(catRepo, auditSvc)
	importSvc := catalogservice.NewBulkImportService(catRepo, auditSvc, "data/imports")
	exportSvc := catalogservice.NewBulkExportService(catRepo, auditSvc, "data/exports")
	catH := cataloghandler.New(catSvc, categorySvc, tagSvc, importSvc, exportSvc)

	// Engagement + Gamification + Search
	gamRepo := gamificationrepo.New(testPool)
	gamSvc := gamificationservice.NewPointsService(gamRepo)
	rankSvc := gamificationservice.NewRankingService(gamRepo)

	engRepo := engagementrepo.New(testPool)
	engSvc := engagementservice.NewEngagementService(engRepo, catRepo, auditSvc, gamSvc)
	engH := engagementhandler.New(engSvc)
	gamH := gamificationhandler.New(gamSvc)

	srchRepo := searchrepo.New(testPool)
	srchSvc := searchservice.NewSearchService(srchRepo)

	strats := []recommendations.RecommendationStrategy{
		recommendations.NewMostEngagedCategories(gamRepo, testPool),
		recommendations.NewFollowedAuthorNewContent(engRepo, testPool),
		recommendations.NewSimilarTagAffinity(gamRepo, testPool),
	}
	recSvc := recommendations.NewRecommendationService(gamRepo, strats)
	srchH := searchhandler.New(srchSvc, rankSvc, recSvc)

	sseHub := sse.NewHub()

	msgRepo := messagingrepo.New(testPool)
	notifSvc := messagingservice.NewNotificationService(msgRepo, sseHub)
	retrySvc := messagingservice.NewRetryService(msgRepo)
	_ = retrySvc
	msgH := messaginghandler.New(notifSvc, sseHub)

	gamSvc.SetNotificationSender(notifSvc)
	catSvc.SetNotificationSender(notifSvc, engRepo)

	modRepo := moderationrepo.New(testPool)
	modSvc := moderationservice.New(modRepo, catRepo, engRepo, gamSvc, auditSvc)
	modSvc.SetNotificationSender(notifSvc)
	modH := moderationhandler.New(modSvc)

	supRepo := supplierrepo.New(testPool)
	supSvc := supplierservice.NewSupplierService(supRepo, auditSvc)
	kpiSvc := supplierservice.NewKPIService(supRepo)
	supH := supplierhandler.New(supSvc, kpiSvc, supRepo)

	analyticsRepo := analyticsrepo.New(testPool)
	analyticsSvc := analyticsservice.NewAnalyticsService(analyticsRepo, auditSvc, "../../data/exports/reports")
	analyticsH := analyticshandler.New(analyticsSvc, auditSvc)

	authH := authhandler.New(authSvc)
	adminH := adminhandler.NewUserHandler(userSvc)

	// Public routes
	r.GET("/", srchH.GetHome)
	r.GET("/search/suggest", srchH.GetSuggest)

	// Auth routes (public)
	r.GET("/login", authH.GetLogin)
	r.GET("/register", authH.GetRegister)
	r.POST("/login", authH.PostLogin)
	r.POST("/register", authH.PostRegister)

	// Protected
	protected := r.Group("")
	protected.Use(middleware.AuthMiddleware(testPool))
	protected.Use(middleware.BanCheckMiddleware(testPool))
	{
		protected.POST("/logout", authH.PostLogout)

		// Resources
		protected.GET("/resources", catH.GetResourceList)
		protected.GET("/resources/new", catH.GetNewResource)
		protected.POST("/resources", middleware.RateLimitMiddleware(testPool, "POST_RESOURCE", 20), catH.PostCreateResource)
		protected.GET("/resources/:id", catH.GetResourceDetail)
		protected.GET("/resources/:id/edit", catH.GetEditResource)
		protected.PUT("/resources/:id", catH.PutUpdateResource)
		protected.DELETE("/resources/:id", catH.DeleteResource)

		// Workflow
		protected.POST("/resources/:id/submit", catH.PostSubmitForReview)
		protected.POST("/resources/:id/revise", catH.PostRevise)

		// Files
		protected.POST("/resources/:id/files", catH.PostUploadFile)
		protected.GET("/resources/:id/files/:fileID", catH.GetDownloadFile)
		protected.DELETE("/resources/:id/files/:fileID", catH.DeleteFile)

		// Tags
		protected.GET("/tags", catH.GetTags)
		protected.POST("/tags", catH.PostCreateTag)

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
		protected.GET("/search/suggest", srchH.GetSuggest)
		protected.GET("/search/history", srchH.GetHistory)
		protected.DELETE("/search/history", srchH.DeleteHistory)

		// Rankings (public-ish but requires auth for recommendations)
		protected.GET("/rankings/bestsellers", srchH.GetBestsellers)
		protected.GET("/rankings/new-releases", srchH.GetNewReleases)
		protected.GET("/recommendations", srchH.GetRecommendations)

		// Reviewer/Admin
		reviewer := protected.Group("")
		reviewer.Use(middleware.RequireRole("REVIEWER", "ADMIN"))
		{
			reviewer.GET("/review-queue", catH.GetReviewQueue)
			reviewer.POST("/resources/:id/approve", catH.PostApprove)
			reviewer.POST("/resources/:id/reject", catH.PostReject)
		}

		// Reports (any authenticated user)
		protected.POST("/reports", modH.CreateReport)

		// Moderation (reviewer + admin)
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

		// Moderation (admin only)
		adminModerationGroup := protected.Group("/moderation")
		adminModerationGroup.Use(middleware.RequireRole("ADMIN"))
		{
			adminModerationGroup.POST("/resources/:id/restore", modH.RestoreResource)
			adminModerationGroup.POST("/users/:id/ban", modH.BanUser)
			adminModerationGroup.POST("/users/:id/unban", modH.UnbanUser)
		}

		// Supplier (SUPPLIER + ADMIN)
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

		// Admin
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
			// Recommendation strategy admin
			adminGroup.GET("/recommendation-strategies", srchH.GetStrategyConfigs)
			adminGroup.PUT("/recommendation-strategies/:id", srchH.PutStrategyConfig)
			// Point rules admin
			adminGroup.GET("/point-rules", gamH.GetPointRules)
			adminGroup.PUT("/point-rules/:id", gamH.PutPointRule)
			// Supplier admin
			adminGroup.GET("/suppliers", supH.GetSupplierList)
			adminGroup.POST("/suppliers", supH.PostCreateSupplier)
			adminGroup.GET("/suppliers/:id", supH.GetSupplierDetail)
			adminGroup.GET("/suppliers/:id/kpis", supH.GetKPIDashboard)
			adminGroup.POST("/suppliers/:id/kpis/recalculate", supH.PostRecalculateKPIs)
			adminGroup.POST("/supplier/orders", supH.PostCreateOrder)
			adminGroup.POST("/supplier/orders/:id/receive", supH.PostConfirmReceipt)
			adminGroup.POST("/supplier/orders/:id/qc", supH.PostSubmitQCResult)
			adminGroup.POST("/supplier/orders/:id/close", supH.PostCloseOrder)
			adminGroup.POST("/supplier/orders/:id/cancel", supH.PostCancelOrder)
			// Analytics admin
			adminGroup.GET("/analytics/reports", analyticsH.GetReportList)
			adminGroup.POST("/analytics/reports/generate", analyticsH.PostGenerateReport)
			adminGroup.GET("/analytics/reports/:id/download", analyticsH.GetReportDownload)
			adminGroup.GET("/audit-logs", analyticsH.GetAuditLogs)
			adminGroup.POST("/audit-logs/export", analyticsH.PostExportAuditLog)
		}

		// Messaging (any authenticated user)
		messagingGroup := protected.Group("/messaging")
		{
			messagingGroup.GET("", msgH.GetCenter)
			messagingGroup.GET("/notifications", msgH.GetNotifications)
			messagingGroup.POST("/notifications/:id/read", msgH.PostMarkRead)
			messagingGroup.POST("/notifications/read-all", msgH.PostMarkAllRead)
			messagingGroup.GET("/subscriptions", msgH.GetSubscriptions)
			messagingGroup.PUT("/subscriptions", msgH.PutSubscriptions)
		}

		// SSE stream
		protected.GET("/events/stream", msgH.GetEventStream)

		// Analytics dashboard (any authenticated user)
		protected.GET("/analytics/dashboard", analyticsH.GetDashboard)
	}

	// Admin routes (auth + role required)
	admin := r.Group("/admin")
	admin.Use(middleware.AuthMiddleware(testPool))
	admin.Use(middleware.RequireRole("ADMIN"))
	{
		admin.GET("/users", adminH.GetUserList)
		admin.GET("/users/:id", adminH.GetUserDetail)
		admin.POST("/users/:id/status", adminH.PostTransitionStatus)
		admin.POST("/users/:id/roles/assign", adminH.PostAssignRole)
		admin.POST("/users/:id/roles/remove", adminH.PostRemoveRole)
		admin.POST("/users/:id/unlock", adminH.PostUnlockUser)
	}

	return r
}

func truncateAll(ctx context.Context) {
	tables := []string{
		// Analytics
		"scheduled_reports", "analytics_summary",
		// Notifications
		"notification_retry_queue", "notification_subscriptions", "notifications",
		// Supplier (before users due to FK)
		"supplier_kpis", "supplier_qc_results", "supplier_asns", "supplier_orders", "suppliers",
		// Moderation (before resources due to FK)
		"user_bans", "moderation_actions", "reports",
		// Engagement
		"anomaly_flags", "follows", "favorites", "votes",
		// Gamification
		"user_badges", "user_points", "point_transactions",
		"ranking_archives",
		// Search
		"user_search_history", "search_terms", "search_index",
		// Catalog
		"resource_reviews", "resource_files", "resource_tags",
		"resource_versions", "bulk_import_jobs", "resources",
		"tags", "categories",
		// Auth
		"sessions", "user_roles", "user_profiles", "users", "audit_logs",
		"rate_limit_counters",
	}
	for _, t := range tables {
		testPool.Exec(ctx, "TRUNCATE TABLE "+t+" CASCADE")
	}
}

// truncate is called per-test to isolate test data.
func truncate(t *testing.T) {
	t.Helper()
	truncateAll(context.Background())
}

// registerUser is a helper that calls the register endpoint.
func registerUser(t *testing.T, username, email, password string) {
	t.Helper()
	repo := authrepo.New(testPool)
	auditSvc := audit.NewService(testPool)
	_ = auditSvc
	svc := authservice.NewAuthService(repo)
	_, err := svc.Register(context.Background(), username, email, password)
	require.NoError(t, err, "helper registerUser failed")
}

// loginUser returns a session token directly via service.
func loginUser(t *testing.T, username, password string) string {
	t.Helper()
	repo := authrepo.New(testPool)
	svc := authservice.NewAuthService(repo)
	result, err := svc.Login(context.Background(), username, password)
	require.NoError(t, err, "helper loginUser failed")
	return result.Token
}

// makeAdmin grants ADMIN role to a user by username.
func makeAdmin(t *testing.T, username string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role, created_at)
		 SELECT id, 'ADMIN', NOW() FROM users WHERE username = $1 ON CONFLICT DO NOTHING`,
		username,
	)
	require.NoError(t, err)
}

// makeAuthor grants AUTHOR role to a user by username.
func makeAuthor(t *testing.T, username string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role, created_at)
		 SELECT id, 'AUTHOR', NOW() FROM users WHERE username = $1 ON CONFLICT DO NOTHING`,
		username,
	)
	require.NoError(t, err)
}

// makeReviewer grants REVIEWER role to a user by username.
func makeReviewer(t *testing.T, username string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role, created_at)
		 SELECT id, 'REVIEWER', NOW() FROM users WHERE username = $1 ON CONFLICT DO NOTHING`,
		username,
	)
	require.NoError(t, err)
}

// sessionCookie returns an http.Cookie for the given session token.
func sessionCookie(token string) *http.Cookie {
	return &http.Cookie{Name: "session_token", Value: token}
}

// authedClient returns an http.Client that always sends the given session cookie.
func authedClient(token string) *http.Client {
	jar := &singleCookieJar{cookie: sessionCookie(token)}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// singleCookieJar is a minimal cookie jar that always sends one cookie.
type singleCookieJar struct {
	cookie *http.Cookie
}

func (j *singleCookieJar) SetCookies(_ *url.URL, _ []*http.Cookie) {}
func (j *singleCookieJar) Cookies(_ *url.URL) []*http.Cookie {
	return []*http.Cookie{j.cookie}
}
