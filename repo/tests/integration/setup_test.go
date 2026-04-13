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

	"github.com/eduexchange/eduexchange/internal/app"
	authrepo "github.com/eduexchange/eduexchange/internal/repository/auth"
	authservice "github.com/eduexchange/eduexchange/internal/service/auth"
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

	// Build test server using the production router (shared constructor)
	testServer = httptest.NewServer(buildRouter())

	code := m.Run()
	testServer.Close()
	os.Exit(code)
}

func buildRouter() *gin.Engine {
	r, _ := app.NewRouter(testPool, []byte("test-encryption-key-32-bytes!!!!"), app.AppDirs{
		Uploads: "data/uploads",
		Imports: "data/imports",
		Exports: "data/exports",
		Reports: "../../data/exports/reports",
	}, time.UTC)
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

// testEncryptionKey is the same 32-byte key used by buildRouter so that
// direct-service helpers and HTTP handlers share the same at-rest encryption.
var testEncryptionKey = []byte("test-encryption-key-32-bytes!!!!")

// registerUser is a helper that registers a user via the auth service directly.
func registerUser(t *testing.T, username, email, password string) {
	t.Helper()
	repo := authrepo.New(testPool)
	svc := authservice.NewAuthService(repo, testEncryptionKey)
	_, err := svc.Register(context.Background(), username, email, password)
	require.NoError(t, err, "helper registerUser failed")
}

// loginUser returns a session token directly via service.
func loginUser(t *testing.T, username, password string) string {
	t.Helper()
	repo := authrepo.New(testPool)
	svc := authservice.NewAuthService(repo, testEncryptionKey)
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
