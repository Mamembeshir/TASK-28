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

	"github.com/eduexchange/eduexchange/internal/app"
	"github.com/eduexchange/eduexchange/internal/audit"
	appcron "github.com/eduexchange/eduexchange/internal/cron"
	"github.com/eduexchange/eduexchange/internal/config"
	appcrypto "github.com/eduexchange/eduexchange/internal/crypto"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	supplierservice "github.com/eduexchange/eduexchange/internal/service/supplier"
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

	log.Println("Waiting for database...")
	pool, err := waitForDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database never became ready: %v", err)
	}
	defer pool.Close()
	log.Println("Database is ready.")

	log.Println("Running migrations...")
	runMigrate()

	var userCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err == nil && userCount == 0 {
		log.Println("Seeding database...")
		runSeed()
	}

	loc, err := time.LoadLocation(cfg.FacilityTZ)
	if err != nil {
		log.Printf("Warning: invalid FACILITY_TIMEZONE %q, falling back to UTC: %v", cfg.FacilityTZ, err)
		loc = time.UTC
	}

	dirs := app.AppDirs{
		Uploads:    cfg.UploadPath,
		Imports:    cfg.UploadPath + "/imports",
		Exports:    cfg.ExportPath,
		Reports:    cfg.ExportPath + "/reports",
		Statements: cfg.StatementPath,
	}
	var r *gin.Engine
	var scheduler *appcron.Scheduler
	if cfg.SecureCookies {
		r, scheduler = app.NewRouterSecure(pool, []byte(cfg.EncryptionKey), dirs, loc)
	} else {
		r, scheduler = app.NewRouter(pool, []byte(cfg.EncryptionKey), dirs, loc)
	}
	r.Static("/static", "./static")
	scheduler.Start()

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

	encKey := os.Getenv("ENCRYPTION_KEY")
	if len(encKey) != 32 {
		log.Fatalf("ENCRYPTION_KEY must be exactly 32 bytes")
	}

	encryptSeedHash := func(raw string) string {
		hash, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash seed password: %v", err)
		}
		encrypted, err := appcrypto.Encrypt([]byte(encKey), hash)
		if err != nil {
			log.Fatalf("Failed to encrypt seed password hash: %v", err)
		}
		return encrypted
	}

	adminPassword := encryptSeedHash("Admin12345!@")
	authorPassword := encryptSeedHash("Author12345!@")
	reviewerPassword := encryptSeedHash("Review12345!@")
	supplierPassword := encryptSeedHash("Supply12345!@")
	userPassword := encryptSeedHash("Teach12345!@")

	type seedUser struct {
		id       uuid.UUID
		username string
		email    string
		password string
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
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, username, email, password_hash, status, failed_login_count, locked_until, version)
			 VALUES ($1, $2, $3, $4, 'ACTIVE', 0, NULL, 1)
			 ON CONFLICT (username) DO UPDATE SET
			   password_hash = EXCLUDED.password_hash,
			   status = 'ACTIVE',
			   failed_login_count = 0,
			   locked_until = NULL,
			   updated_at = NOW()`,
			u.id, u.username, u.email, u.password,
		)
		if err != nil {
			log.Printf("Warning: failed to upsert user %s: %v", u.username, err)
			continue
		}

		var actualID uuid.UUID
		err = pool.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, u.username).Scan(&actualID)
		if err != nil {
			continue
		}

		for _, role := range u.roles {
			pool.Exec(ctx, //nolint:errcheck
				`INSERT INTO user_roles (user_id, role) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				actualID, role,
			)
		}

		pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO user_profiles (user_id, display_name) VALUES ($1, $2)
			 ON CONFLICT (user_id) DO UPDATE SET display_name = EXCLUDED.display_name`,
			actualID, u.display,
		)
	}

	// Seed a supplier record linked to supplier1 if not already linked.
	// Contact is encrypted at rest via AES-256-GCM using the ENCRYPTION_KEY env var.
	var supplier1ID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE username = 'supplier1'`).Scan(&supplier1ID); err == nil {
		var existingCount int
		pool.QueryRow(ctx, `SELECT COUNT(*) FROM suppliers WHERE user_id = $1`, supplier1ID).Scan(&existingCount) //nolint:errcheck
		if existingCount == 0 {
			encKey := os.Getenv("ENCRYPTION_KEY")
			if len(encKey) != 32 {
				log.Printf("Seed warning: ENCRYPTION_KEY must be 32 bytes; skipping supplier seed")
			} else {
				auditSvc := audit.NewService(pool)
				supplierSvc := supplierservice.NewSupplierService(supplierrepo.New(pool), auditSvc, []byte(encKey))
				supplier, err := supplierSvc.CreateSupplier(ctx,
					supplier1ID, // actor = the supplier user being seeded
					"Demo Supplier Co.",
					`{"email":"supplier1@eduexchange.local","phone":"555-0100"}`,
					"supplier1@...",
				)
				if err != nil {
					log.Printf("Seed warning: failed to create supplier: %v", err)
				} else {
					pool.Exec(ctx, //nolint:errcheck
						`UPDATE suppliers SET user_id = $1, tier = 'SILVER' WHERE id = $2`,
						supplier1ID, supplier.ID,
					)
				}
			}
		}
	}

	log.Println("Seed data created successfully")
}
