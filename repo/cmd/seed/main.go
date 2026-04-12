// Package main provides a seed command to populate the database with
// realistic demo data for development and manual testing.
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./cmd/seed
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	authrepo "github.com/eduexchange/eduexchange/internal/repository/auth"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	messagingrepo "github.com/eduexchange/eduexchange/internal/repository/messaging"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	authservice "github.com/eduexchange/eduexchange/internal/service/auth"
	catalogservice "github.com/eduexchange/eduexchange/internal/service/catalog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://eduexchange:eduexchange@localhost:5432/eduexchange?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping: %v", err)
	}

	s := &seeder{
		ctx:        ctx,
		pool:       pool,
		authRepo:   authrepo.New(pool),
		catRepo:    catalogrepo.New(pool),
		engRepo:    engagementrepo.New(pool),
		gamRepo:    gamificationrepo.New(pool),
		supRepo:    supplierrepo.New(pool),
		msgRepo:    messagingrepo.New(pool),
		auditSvc:   audit.NewService(pool),
	}

	s.authSvc = authservice.NewAuthService(s.authRepo)
	s.catSvc = catalogservice.NewCatalogService(s.catRepo, s.auditSvc, "data/uploads")

	log.Println("seeding database...")
	if err := s.run(); err != nil {
		log.Fatalf("seed failed: %v", err)
	}
	log.Println("seed complete")
}

type seeder struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	authRepo authrepo.UserRepository
	catRepo  catalogrepo.CatalogRepository
	engRepo  engagementrepo.EngagementRepository
	gamRepo  gamificationrepo.GamificationRepository
	supRepo  supplierrepo.SupplierRepository
	msgRepo  messagingrepo.MessagingRepository
	auditSvc *audit.Service
	authSvc  *authservice.AuthService
	catSvc   *catalogservice.CatalogService
}

func (s *seeder) run() error {
	// ── 1. Users ──────────────────────────────────────────────────────────────
	log.Println("  creating users...")
	users := []struct {
		username string
		email    string
		password string
		roles    []string
	}{
		{"admin", "admin@eduexchange.local", "Admin12345!@", []string{"ADMIN"}},
		{"author1", "author1@eduexchange.local", "Author12345!@", []string{"AUTHOR"}},
		{"reviewer1", "reviewer1@eduexchange.local", "Review12345!@", []string{"REVIEWER"}},
		{"supplier1", "supplier1@eduexchange.local", "Supply12345!@", []string{"SUPPLIER"}},
		{"teacher1", "teacher1@eduexchange.local", "Teach12345!@", []string{}},
	}

	userIDs := map[string]uuid.UUID{}
	for _, u := range users {
		existing, _ := s.authRepo.FindByUsername(s.ctx, u.username)
		if existing != nil {
			userIDs[u.username] = existing.ID
			log.Printf("    %s already exists, skipping", u.username)
			continue
		}

		user, err := s.authSvc.Register(s.ctx, u.username, u.email, u.password)
		if err != nil {
			return fmt.Errorf("register %s: %w", u.username, err)
		}
		userIDs[u.username] = user.ID

		for _, role := range u.roles {
			_, err := s.pool.Exec(s.ctx,
				`INSERT INTO user_roles (user_id, role, created_at)
				 VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING`,
				user.ID, role)
			if err != nil {
				return fmt.Errorf("assign role %s to %s: %w", role, u.username, err)
			}
		}
		log.Printf("    created %s (%s)", u.username, user.ID)
	}

	authorID := userIDs["author1"]
	adminID := userIDs["admin"]
	reviewerID := userIDs["reviewer1"]
	teacher1ID := userIDs["teacher1"]

	// ── 2. Categories ─────────────────────────────────────────────────────────
	log.Println("  creating categories...")
	catMap := s.seedCategories()

	// ── 3. Tags ───────────────────────────────────────────────────────────────
	log.Println("  creating tags...")
	s.seedTags()

	// ── 4. Resources ──────────────────────────────────────────────────────────
	log.Println("  creating resources...")
	resources := s.seedResources(authorID, adminID, reviewerID, catMap)

	// ── 5. Engagement ─────────────────────────────────────────────────────────
	log.Println("  creating engagement data...")
	s.seedEngagement(teacher1ID, resources)

	// ── 6. Gamification ───────────────────────────────────────────────────────
	log.Println("  seeding gamification points...")
	s.seedGamification(authorID)

	// ── 7. Suppliers ──────────────────────────────────────────────────────────
	log.Println("  creating suppliers and orders...")
	s.seedSuppliers(adminID)

	// ── 8. Notifications ──────────────────────────────────────────────────────
	log.Println("  creating notifications...")
	s.seedNotifications(authorID, teacher1ID)

	// ── 9. Notification subscriptions ─────────────────────────────────────────
	log.Println("  seeding notification subscriptions...")
	s.seedSubscriptions(teacher1ID)

	// ── 10. Search terms / synonyms ───────────────────────────────────────────
	log.Println("  seeding search terms and synonyms...")
	s.seedSearch()

	// ── 11. Recommendations ───────────────────────────────────────────────────
	log.Println("  seeding recommendation strategies...")
	s.seedRecommendationStrategies()

	return nil
}

func (s *seeder) seedCategories() map[string]uuid.UUID {
	type catSpec struct {
		name   string
		parent string
	}
	specs := []catSpec{
		{"Mathematics", ""},
		{"Algebra", "Mathematics"},
		{"Geometry", "Mathematics"},
		{"Calculus", "Mathematics"},
		{"Science", ""},
		{"Biology", "Science"},
		{"Chemistry", "Science"},
		{"Physics", "Science"},
		{"Language Arts", ""},
		{"Grammar", "Language Arts"},
		{"Essay Writing", "Language Arts"},
		{"Literature", "Language Arts"},
		{"Social Studies", ""},
	}

	catMap := map[string]uuid.UUID{}
	for _, spec := range specs {
		// Check existing
		var id uuid.UUID
		err := s.pool.QueryRow(s.ctx, `SELECT id FROM categories WHERE name=$1`, spec.name).Scan(&id)
		if err == nil {
			catMap[spec.name] = id
			continue
		}

		id = uuid.New()
		var parentID *uuid.UUID
		if spec.parent != "" {
			pid := catMap[spec.parent]
			parentID = &pid
		}
		_, err = s.pool.Exec(s.ctx,
			`INSERT INTO categories (id, name, parent_id, sort_order, created_at, updated_at)
			 VALUES ($1, $2, $3, 0, NOW(), NOW()) ON CONFLICT (name) DO NOTHING`,
			id, spec.name, parentID)
		if err != nil {
			log.Printf("    category %s: %v", spec.name, err)
			continue
		}
		catMap[spec.name] = id
	}
	return catMap
}

func (s *seeder) seedTags() {
	tags := []string{"algebra", "geometry", "biology", "chemistry", "grammar", "essay", "history"}
	for _, name := range tags {
		s.pool.Exec(s.ctx, //nolint:errcheck
			`INSERT INTO tags (id, name, created_at) VALUES (gen_random_uuid(), $1, NOW())
			 ON CONFLICT (name) DO NOTHING`, name)
	}
}

func (s *seeder) seedResources(authorID, adminID, reviewerID uuid.UUID, catMap map[string]uuid.UUID) []uuid.UUID {
	type resSpec struct {
		title       string
		description string
		status      string
		categoryKey string
	}

	algID := catMap["Algebra"]
	geoID := catMap["Geometry"]
	bioID := catMap["Biology"]

	specs := []resSpec{
		{"Introduction to Algebra", "A beginner's guide to algebra", "PUBLISHED", "Algebra"},
		{"Advanced Geometry", "Theorems and proofs in geometry", "PUBLISHED", "Geometry"},
		{"Cell Biology Fundamentals", "Understanding cells", "PUBLISHED", "Biology"},
		{"Essay Writing Techniques", "How to write compelling essays", "PUBLISHED", "Essay Writing"},
		{"World History: Ancient Civilizations", "From Mesopotamia to Rome", "PUBLISHED", "Social Studies"},
		{"Calculus for Beginners", "Derivatives and integrals", "APPROVED", "Calculus"},
		{"Chemistry Reactions", "Balancing chemical equations", "APPROVED", "Chemistry"},
		{"Grammar Essentials", "Parts of speech and sentence structure", "APPROVED", "Grammar"},
		{"Algebra Problem Sets", "Practice problems for algebra", "PENDING_REVIEW", "Algebra"},
		{"Physics Mechanics", "Newton's laws and beyond", "PENDING_REVIEW", "Physics"},
		{"Draft: Statistics 101", "An introduction to statistics", "DRAFT", "Mathematics"},
		{"Draft: Literature Analysis", "Analyzing great works", "DRAFT", "Language Arts"},
		{"Rejected: Poor Quality", "This needs work", "REJECTED", "Mathematics"},
		{"Rejected: Incomplete", "Missing content", "REJECTED", "Science"},
		{"Taken Down: Copyright", "This was taken down", "TAKEN_DOWN", "Mathematics"},
	}

	_ = algID
	_ = geoID
	_ = bioID

	var resourceIDs []uuid.UUID
	for _, spec := range specs {
		// Check if exists
		var count int
		s.pool.QueryRow(s.ctx, `SELECT COUNT(*) FROM resources WHERE title=$1`, spec.title).Scan(&count) //nolint:errcheck
		if count > 0 {
			var existingID uuid.UUID
			s.pool.QueryRow(s.ctx, `SELECT id FROM resources WHERE title=$1`, spec.title).Scan(&existingID) //nolint:errcheck
			resourceIDs = append(resourceIDs, existingID)
			continue
		}

		catID := catMap[spec.categoryKey]
		res, err := s.catSvc.CreateDraft(s.ctx, authorID, catalogservice.ResourceInput{
			Title:       spec.title,
			Description: spec.description,
			ContentBody: "Content for " + spec.title,
			CategoryID:  &catID,
		})
		if err != nil {
			log.Printf("    resource %s: %v", spec.title, err)
			continue
		}
		resourceIDs = append(resourceIDs, res.ID)

		// Transition to target status
		switch spec.status {
		case "PENDING_REVIEW":
			ver := res.CurrentVersionNumber
			s.catSvc.SubmitForReview(s.ctx, res.ID, authorID, ver) //nolint:errcheck
		case "APPROVED":
			ver := res.CurrentVersionNumber
			s.catSvc.SubmitForReview(s.ctx, res.ID, authorID, ver) //nolint:errcheck
			ver++
			s.catSvc.Approve(s.ctx, res.ID, reviewerID, ver) //nolint:errcheck
		case "PUBLISHED":
			ver := res.CurrentVersionNumber
			s.catSvc.SubmitForReview(s.ctx, res.ID, authorID, ver) //nolint:errcheck
			ver++
			s.catSvc.Approve(s.ctx, res.ID, reviewerID, ver) //nolint:errcheck
			ver++
			s.catSvc.Publish(s.ctx, res.ID, adminID, ver) //nolint:errcheck
		case "REJECTED":
			ver := res.CurrentVersionNumber
			s.catSvc.SubmitForReview(s.ctx, res.ID, authorID, ver) //nolint:errcheck
			ver++
			s.catSvc.Reject(s.ctx, res.ID, reviewerID, "Needs improvement", ver) //nolint:errcheck
		case "TAKEN_DOWN":
			ver := res.CurrentVersionNumber
			s.catSvc.SubmitForReview(s.ctx, res.ID, authorID, ver) //nolint:errcheck
			ver++
			s.catSvc.Approve(s.ctx, res.ID, reviewerID, ver) //nolint:errcheck
			ver++
			s.catSvc.Publish(s.ctx, res.ID, adminID, ver) //nolint:errcheck
			ver++
			// Direct DB update for takedown to avoid audit cycle
			s.pool.Exec(s.ctx, `UPDATE resources SET status='TAKEN_DOWN' WHERE id=$1`, res.ID) //nolint:errcheck
		}
	}

	return resourceIDs
}

func (s *seeder) seedEngagement(userID uuid.UUID, resourceIDs []uuid.UUID) {
	// Votes on published resources (first 5)
	for i, rid := range resourceIDs {
		if i >= 5 {
			break
		}
		dir := model.VoteTypeUp
		if i == 4 {
			dir = model.VoteTypeDown
		}
		s.engRepo.UpsertVote(s.ctx, &model.Vote{ //nolint:errcheck
			ID: uuid.New(), UserID: userID, ResourceID: rid, VoteType: dir,
		})
	}

	// Favorites on first 3
	for i, rid := range resourceIDs {
		if i >= 3 {
			break
		}
		s.engRepo.UpsertFavorite(s.ctx, &model.Favorite{ //nolint:errcheck
			ID: uuid.New(), UserID: userID, ResourceID: rid,
		})
	}

	// Follow author1
	authorUser, _ := s.authRepo.FindByUsername(s.ctx, "author1")
	if authorUser != nil {
		s.engRepo.UpsertFollow(s.ctx, &model.Follow{ //nolint:errcheck
			ID: uuid.New(), FollowerID: userID, TargetType: model.FollowTargetAuthor, TargetID: authorUser.ID,
		})
	}
}

func (s *seeder) seedGamification(authorID uuid.UUID) {
	// Author at level 2 with 420 points and 3 badges
	s.pool.Exec(s.ctx, //nolint:errcheck
		`INSERT INTO user_points (user_id, total_points, level, created_at, updated_at)
		 VALUES ($1, 420, 2, NOW(), NOW())
		 ON CONFLICT (user_id) DO UPDATE SET total_points=420, level=2, updated_at=NOW()`,
		authorID)

	badges := []string{"first_resource", "popular_author", "top_contributor"}
	for _, badge := range badges {
		s.pool.Exec(s.ctx, //nolint:errcheck
			`INSERT INTO user_badges (id, user_id, badge_type, awarded_at)
			 VALUES (gen_random_uuid(), $1, $2, NOW())
			 ON CONFLICT DO NOTHING`,
			authorID, badge)
	}
}

func (s *seeder) seedSuppliers(adminID uuid.UUID) {
	_ = adminID

	// Create 2 suppliers
	suppliers := []struct {
		name  string
		email string
		tier  string
	}{
		{"Gold Supplier Co.", "gold@supplier.com", "GOLD"},
		{"Bronze Supplier Ltd.", "bronze@supplier.com", "BRONZE"},
	}

	for _, sup := range suppliers {
		var count int
		s.pool.QueryRow(s.ctx, `SELECT COUNT(*) FROM suppliers WHERE name=$1`, sup.name).Scan(&count) //nolint:errcheck
		if count > 0 {
			continue
		}

		supID := uuid.New()
		s.pool.Exec(s.ctx, //nolint:errcheck
			`INSERT INTO suppliers (id, name, contact_email, tier, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'ACTIVE', NOW(), NOW())`,
			supID, sup.name, sup.email, sup.tier)

		// Create 2-3 orders per supplier in various states
		statuses := []string{"OPEN", "CONFIRMED", "CLOSED"}
		for _, status := range statuses {
			orderID := uuid.New()
			s.pool.Exec(s.ctx, //nolint:errcheck
				`INSERT INTO supplier_orders (id, supplier_id, item_name, quantity, unit_price, status, created_at, updated_at)
				 VALUES ($1, $2, $3, 50, 25.00, $4, NOW(), NOW())`,
				orderID, supID, "Educational Materials ("+status+")", status)
		}
	}
}

func (s *seeder) seedNotifications(authorID, teacher1ID uuid.UUID) {
	type notifSpec struct {
		userID    uuid.UUID
		eventType model.EventType
		title     string
		body      string
		isRead    bool
	}

	specs := []notifSpec{
		{authorID, model.EventReviewDecision, "Resource Approved", "Your resource was approved", false},
		{authorID, model.EventPublishComplete, "Resource Published", "Your resource is now live", true},
		{authorID, model.EventBadgeEarned, "Badge Earned", "You earned the Popular Author badge", false},
		{authorID, model.EventLevelUp, "Level Up!", "You reached level 2", true},
		{teacher1ID, model.EventFollowNewContent, "New Content", "Author1 published a new resource", false},
	}

	for _, spec := range specs {
		var count int
		s.pool.QueryRow(s.ctx, //nolint:errcheck
			`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND event_type=$2 AND title=$3`,
			spec.userID, spec.eventType, spec.title).Scan(&count)
		if count > 0 {
			continue
		}

		notifID := uuid.New()
		s.pool.Exec(s.ctx, //nolint:errcheck
			`INSERT INTO notifications (id, user_id, event_type, title, body, is_read, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW())`,
			notifID, spec.userID, string(spec.eventType), spec.title, spec.body, spec.isRead)
	}
}

func (s *seeder) seedSubscriptions(teacher1ID uuid.UUID) {
	// teacher1 has review_decision disabled
	s.pool.Exec(s.ctx, //nolint:errcheck
		`INSERT INTO notification_subscriptions (user_id, event_type, enabled, updated_at)
		 VALUES ($1, 'review_decision', false, NOW())
		 ON CONFLICT (user_id, event_type) DO UPDATE SET enabled=false, updated_at=NOW()`,
		teacher1ID)
}

func (s *seeder) seedSearch() {
	// Synonym groups
	synonymGroups := [][]string{
		{"math", "mathematics", "maths"},
		{"bio", "biology", "life science"},
		{"english", "language arts", "grammar"},
	}
	for _, terms := range synonymGroups {
		s.pool.Exec(s.ctx, //nolint:errcheck
			`INSERT INTO synonym_groups (id, terms, created_at) VALUES (gen_random_uuid(), $1, NOW())
			 ON CONFLICT DO NOTHING`, terms)
	}

	// Common search terms
	searchTerms := []string{"algebra", "geometry", "biology", "grammar", "history"}
	for _, term := range searchTerms {
		s.pool.Exec(s.ctx, //nolint:errcheck
			`INSERT INTO search_terms (id, term, frequency, created_at)
			 VALUES (gen_random_uuid(), $1, 10, NOW())
			 ON CONFLICT (term) DO UPDATE SET frequency=search_terms.frequency+1`,
			term)
	}
}

func (s *seeder) seedRecommendationStrategies() {
	strategies := []struct {
		id      string
		name    string
		weight  float64
		enabled bool
	}{
		{"most_engaged_categories", "MostEngagedCategories", 1.0, true},
		{"followed_author_new_content", "FollowedAuthorNewContent", 1.5, true},
		{"similar_tag_affinity", "SimilarTagAffinity", 1.2, true},
	}

	for _, strat := range strategies {
		s.pool.Exec(s.ctx, //nolint:errcheck
			`INSERT INTO recommendation_strategies (id, name, weight, enabled, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, NOW(), NOW())
			 ON CONFLICT (id) DO UPDATE SET weight=$3, enabled=$4, updated_at=NOW()`,
			strat.id, strat.name, strat.weight, strat.enabled)
	}
}

// adminPlaceholderID returns a zero UUID used as a placeholder actor in seed operations.
func adminPlaceholderID() uuid.UUID {
	return uuid.Nil
}

func init() {
	// Silence unused import warning for time
	_ = time.Now
}
