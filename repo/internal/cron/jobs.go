package cron

import (
	"context"
	"log"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	analyticsservice "github.com/eduexchange/eduexchange/internal/service/analytics"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
	messagingservice "github.com/eduexchange/eduexchange/internal/service/messaging"
	supplierservice "github.com/eduexchange/eduexchange/internal/service/supplier"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	robfigcron "github.com/robfig/cron/v3"
)

// Scheduler wraps the cron scheduler with all registered jobs.
type Scheduler struct {
	c            *robfigcron.Cron
	rankSvc      *gamificationservice.RankingService
	engRepo      engagementrepo.EngagementRepository
	pool         *pgxpool.Pool
	kpiSvc       *supplierservice.KPIService
	supplierRepo supplierrepo.SupplierRepository
	retrySvc     *messagingservice.RetryService
	analyticsSvc *analyticsservice.AnalyticsService
}

// New creates and starts the scheduler.
// loc sets the timezone used for scheduling (nil → UTC).
func New(
	rankSvc *gamificationservice.RankingService,
	engRepo engagementrepo.EngagementRepository,
	pool *pgxpool.Pool,
	kpiSvc *supplierservice.KPIService,
	supplierRepo supplierrepo.SupplierRepository,
	retrySvc *messagingservice.RetryService,
	analyticsSvc *analyticsservice.AnalyticsService,
	loc *time.Location,
) *Scheduler {
	if loc == nil {
		loc = time.UTC
	}
	c := robfigcron.New(robfigcron.WithSeconds(), robfigcron.WithLocation(loc))
	s := &Scheduler{c: c, rankSvc: rankSvc, engRepo: engRepo, pool: pool, kpiSvc: kpiSvc, supplierRepo: supplierRepo, retrySvc: retrySvc, analyticsSvc: analyticsSvc}

	// WeeklyRankingReset — Monday 02:00 AM local (RANK-03)
	// Cron: "0 0 2 * * 1" = second minute hour dom month weekday
	if _, err := c.AddFunc("0 0 2 * * 1", s.weeklyRankingReset); err != nil {
		log.Printf("cron: failed to register WeeklyRankingReset: %v", err)
	}

	// LikeRingDetection — every 6 hours (MOD-02)
	if _, err := c.AddFunc("0 0 */6 * * *", s.likeRingDetection); err != nil {
		log.Printf("cron: failed to register LikeRingDetection: %v", err)
	}

	// CleanIdempotencyRecords — daily 03:00 AM
	if _, err := c.AddFunc("0 0 3 * * *", s.cleanIdempotencyRecords); err != nil {
		log.Printf("cron: failed to register CleanIdempotencyRecords: %v", err)
	}

	// CleanExpiredSessions — daily 04:00 AM
	if _, err := c.AddFunc("0 0 4 * * *", s.cleanExpiredSessions); err != nil {
		log.Printf("cron: failed to register CleanExpiredSessions: %v", err)
	}

	// CleanRateLimitCounters — daily 05:00 AM
	if _, err := c.AddFunc("0 0 5 * * *", s.cleanRateLimitCounters); err != nil {
		log.Printf("cron: failed to register CleanRateLimitCounters: %v", err)
	}

	// SupplierKPIRecalculation — nightly 01:00 AM
	if _, err := c.AddFunc("0 0 1 * * *", s.supplierKPIRecalculation); err != nil {
		log.Printf("cron: failed to register SupplierKPIRecalculation: %v", err)
	}

	// DeliveryConfirmationEscalation — every 4 hours
	if _, err := c.AddFunc("0 0 */4 * * *", s.deliveryConfirmationEscalation); err != nil {
		log.Printf("cron: failed to register DeliveryConfirmationEscalation: %v", err)
	}

	// QCResultEscalation — every 4 hours
	if _, err := c.AddFunc("0 30 */4 * * *", s.qcResultEscalation); err != nil {
		log.Printf("cron: failed to register QCResultEscalation: %v", err)
	}

	// NotificationRetryProcessor — every 1 minute
	if _, err := c.AddFunc("0 * * * * *", s.notificationRetryProcessor); err != nil {
		log.Printf("cron: failed to register NotificationRetryProcessor: %v", err)
	}

	// AnalyticsRefresh — every 15 minutes
	if _, err := c.AddFunc("0 */15 * * * *", s.analyticsRefresh); err != nil {
		log.Printf("cron: failed to register AnalyticsRefresh: %v", err)
	}

	return s
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	s.c.Start()
	log.Println("cron: scheduler started")
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.c.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(30 * time.Second):
		log.Println("cron: timed out waiting for jobs to finish")
	}
	log.Println("cron: scheduler stopped")
}

func (s *Scheduler) weeklyRankingReset() {
	ctx := context.Background()
	log.Println("cron: running WeeklyRankingReset")
	if err := s.rankSvc.WeeklyReset(ctx); err != nil {
		log.Printf("cron: WeeklyRankingReset error: %v", err)
	}
}

func (s *Scheduler) likeRingDetection() {
	ctx := context.Background()
	log.Println("cron: running LikeRingDetection")

	// Query for reciprocal pairs: both A→B and B→A voted in last 24h,
	// each direction having count > 15.  This avoids false positives from
	// one-way bulk voting.
	rows, err := s.pool.Query(ctx, `
		WITH directional AS (
			SELECT v.user_id AS voter, r.author_id AS author, COUNT(*) AS cnt
			FROM votes v
			JOIN resources r ON r.id = v.resource_id
			WHERE v.updated_at >= NOW() - INTERVAL '24 hours'
			  AND v.user_id != r.author_id
			GROUP BY v.user_id, r.author_id
			HAVING COUNT(*) > 15
		)
		SELECT a.voter AS user_a, a.author AS user_b, a.cnt + b.cnt AS cnt
		FROM directional a
		JOIN directional b ON a.voter = b.author AND a.author = b.voter
		WHERE a.voter < a.author`)
	if err != nil {
		log.Printf("cron: LikeRingDetection query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userA, userB uuid.UUID
		var cnt int
		if err := rows.Scan(&userA, &userB, &cnt); err != nil {
			continue
		}
		log.Printf("cron: LikeRingDetection potential ring: %s <-> %s (%d votes)", userA, userB, cnt)
		flag := &model.AnomalyFlag{
			ID:       uuid.New(),
			FlagType: "LIKE_RING",
			UserIDs:  []uuid.UUID{userA, userB},
			EvidenceJSON: map[string]interface{}{
				"mutual_votes_24h": cnt,
				"user_a":           userA.String(),
				"user_b":           userB.String(),
			},
			Status: "OPEN",
		}
		_ = s.engRepo.CreateAnomalyFlag(ctx, flag)
	}
}

func (s *Scheduler) cleanIdempotencyRecords() {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM idempotency_records WHERE created_at < NOW() - INTERVAL '24 hours'`)
	if err != nil {
		log.Printf("cron: CleanIdempotencyRecords error: %v", err)
		return
	}
	log.Printf("cron: CleanIdempotencyRecords deleted %d rows", tag.RowsAffected())
}

func (s *Scheduler) cleanExpiredSessions() {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE expires_at < NOW()`)
	if err != nil {
		log.Printf("cron: CleanExpiredSessions error: %v", err)
		return
	}
	log.Printf("cron: CleanExpiredSessions deleted %d rows", tag.RowsAffected())
}

// RunLikeRingDetection is a public method for testing the like-ring detection logic.
func (s *Scheduler) RunLikeRingDetection() {
	s.likeRingDetection()
}

func (s *Scheduler) cleanRateLimitCounters() {
	// Delete counters older than 2 hours (keep current window and previous)
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM rate_limit_counters WHERE window_start < NOW() - INTERVAL '2 hours'`)
	if err != nil {
		log.Printf("cron: CleanRateLimitCounters error: %v", err)
		return
	}
	log.Printf("cron: CleanRateLimitCounters deleted %d rows", tag.RowsAffected())
}

func (s *Scheduler) supplierKPIRecalculation() {
	ctx := context.Background()
	log.Println("cron: running SupplierKPIRecalculation")
	if s.kpiSvc == nil {
		return
	}
	if err := s.kpiSvc.RecalculateAllKPIs(ctx); err != nil {
		log.Printf("cron: SupplierKPIRecalculation error: %v", err)
	}
}

func (s *Scheduler) deliveryConfirmationEscalation() {
	ctx := context.Background()
	log.Println("cron: running DeliveryConfirmationEscalation")
	if s.supplierRepo == nil || s.engRepo == nil {
		return
	}

	// Flag orders in CREATED status older than 48h (standard deadline)
	deadline := time.Now().UTC().Add(-48 * time.Hour)
	orders, err := s.supplierRepo.GetOrdersAwaitingConfirmation(ctx, deadline)
	if err != nil {
		log.Printf("cron: DeliveryConfirmationEscalation query error: %v", err)
		return
	}

	for _, order := range orders {
		log.Printf("cron: DeliveryConfirmationEscalation: order %s needs escalation", order.ID)
		flag := &model.AnomalyFlag{
			ID:       uuid.New(),
			FlagType: "OTHER",
			UserIDs:  []uuid.UUID{},
			EvidenceJSON: map[string]interface{}{
				"type":         "delivery_confirmation_overdue",
				"order_id":     order.ID.String(),
				"order_number": order.OrderNumber,
				"supplier_id":  order.SupplierID.String(),
				"created_at":   order.CreatedAt.Format(time.RFC3339),
			},
			Status: "OPEN",
		}
		if err := s.engRepo.CreateAnomalyFlag(ctx, flag); err != nil {
			log.Printf("cron: DeliveryConfirmationEscalation flag error: %v", err)
		}
	}
}

func (s *Scheduler) qcResultEscalation() {
	ctx := context.Background()
	log.Println("cron: running QCResultEscalation")
	if s.supplierRepo == nil || s.engRepo == nil {
		return
	}

	// Flag orders in RECEIVED status for more than 24h without QC
	deadline := time.Now().UTC().Add(-24 * time.Hour)
	orders, err := s.supplierRepo.GetOrdersAwaitingQC(ctx, deadline)
	if err != nil {
		log.Printf("cron: QCResultEscalation query error: %v", err)
		return
	}

	for _, order := range orders {
		log.Printf("cron: QCResultEscalation: order %s needs QC escalation", order.ID)
		flag := &model.AnomalyFlag{
			ID:       uuid.New(),
			FlagType: "OTHER",
			UserIDs:  []uuid.UUID{},
			EvidenceJSON: map[string]interface{}{
				"type":         "qc_result_overdue",
				"order_id":     order.ID.String(),
				"order_number": order.OrderNumber,
				"supplier_id":  order.SupplierID.String(),
				"received_at":  order.ReceivedAt,
			},
			Status: "OPEN",
		}
		if err := s.engRepo.CreateAnomalyFlag(ctx, flag); err != nil {
			log.Printf("cron: QCResultEscalation flag error: %v", err)
		}
	}
}

// RunDeliveryEscalation is a public method for testing the delivery escalation logic.
func (s *Scheduler) RunDeliveryEscalation() {
	s.deliveryConfirmationEscalation()
}

// RunQCEscalation is a public method for testing the QC escalation logic.
func (s *Scheduler) RunQCEscalation() {
	s.qcResultEscalation()
}

func (s *Scheduler) notificationRetryProcessor() {
	ctx := context.Background()
	log.Println("cron: running NotificationRetryProcessor")
	if s.retrySvc == nil {
		return
	}
	if err := s.retrySvc.ProcessRetryQueue(ctx); err != nil {
		log.Printf("cron: NotificationRetryProcessor error: %v", err)
	}
}

func (s *Scheduler) analyticsRefresh() {
	ctx := context.Background()
	log.Println("cron: running AnalyticsRefresh")
	if s.analyticsSvc == nil {
		return
	}
	if err := s.analyticsSvc.RefreshMetrics(ctx); err != nil {
		log.Printf("cron: AnalyticsRefresh error: %v", err)
	}
}

// RunNotificationRetry is a public method for testing the notification retry logic.
func (s *Scheduler) RunNotificationRetry() {
	s.notificationRetryProcessor()
}

// RunAnalyticsRefresh is a public method for testing the analytics refresh logic.
func (s *Scheduler) RunAnalyticsRefresh() {
	s.analyticsRefresh()
}
