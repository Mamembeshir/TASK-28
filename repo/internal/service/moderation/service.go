package moderationservice

import (
	"context"
	"fmt"
	"time"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	moderationrepo "github.com/eduexchange/eduexchange/internal/repository/moderation"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
	"github.com/google/uuid"
)

// GamificationHooks is the interface for awarding/deducting points.
type GamificationHooks interface {
	AwardPoints(ctx context.Context, userID uuid.UUID, eventType string, sourceID *uuid.UUID) error
}

// NotificationSender is the interface for sending notifications from the moderation service.
type NotificationSender interface {
	Send(ctx context.Context, userID uuid.UUID, eventType model.EventType, title, body string, resourceID *uuid.UUID) error
}

// ModerationService handles moderation actions.
type ModerationService struct {
	modRepo  moderationrepo.ModerationRepository
	catRepo  catalogrepo.CatalogRepository
	engRepo  engagementrepo.EngagementRepository
	gamSvc   *gamificationservice.PointsService
	auditSvc *audit.Service
	notifSvc NotificationSender
}

// New creates a new ModerationService.
func New(
	modRepo moderationrepo.ModerationRepository,
	catRepo catalogrepo.CatalogRepository,
	engRepo engagementrepo.EngagementRepository,
	gamSvc *gamificationservice.PointsService,
	auditSvc *audit.Service,
) *ModerationService {
	return &ModerationService{
		modRepo:  modRepo,
		catRepo:  catRepo,
		engRepo:  engRepo,
		gamSvc:   gamSvc,
		auditSvc: auditSvc,
	}
}

// SetNotificationSender wires in the notification service after construction.
func (s *ModerationService) SetNotificationSender(n NotificationSender) {
	s.notifSvc = n
}

// CreateReport creates a new report on a published resource.
func (s *ModerationService) CreateReport(ctx context.Context, reporterID uuid.UUID, resourceID uuid.UUID, reasonType, description string) (*model.Report, error) {
	// Validate resource exists and is PUBLISHED
	res, err := s.catRepo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("resource not found: %w", err)
	}
	if res.Status != model.ResourceStatusPublished {
		return nil, fmt.Errorf("%w: resource is not published", model.ErrValidation)
	}

	report := &model.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ResourceID:  resourceID,
		ReasonType:  reasonType,
		Description: description,
		Status:      "OPEN",
		Notes:       "",
	}
	if err := s.modRepo.CreateReport(ctx, report); err != nil {
		return nil, err
	}
	return s.modRepo.GetReport(ctx, report.ID)
}

// AssignReport transitions a report from OPEN to UNDER_REVIEW and assigns a reviewer.
func (s *ModerationService) AssignReport(ctx context.Context, reportID, reviewerID uuid.UUID) error {
	report, err := s.modRepo.GetReport(ctx, reportID)
	if err != nil {
		return err
	}
	if report.Status != "OPEN" {
		return fmt.Errorf("%w: report is not OPEN", model.ErrValidation)
	}
	report.Status = "UNDER_REVIEW"
	report.ReviewerID = &reviewerID
	return s.modRepo.UpdateReport(ctx, report)
}

// ResolveReport transitions a report to RESOLVED and records a moderation action.
func (s *ModerationService) ResolveReport(ctx context.Context, reportID, moderatorID uuid.UUID, actionType, notes, evidence string) error {
	report, err := s.modRepo.GetReport(ctx, reportID)
	if err != nil {
		return err
	}
	report.Status = "RESOLVED"
	report.Notes = notes
	if err := s.modRepo.UpdateReport(ctx, report); err != nil {
		return err
	}

	evidenceMap := map[string]interface{}{"evidence": evidence}
	action := &model.ModerationAction{
		ID:           uuid.New(),
		ModeratorID:  moderatorID,
		ActionType:   actionType,
		TargetType:   "RESOURCE",
		TargetID:     report.ResourceID,
		ReportID:     &reportID,
		Notes:        notes,
		EvidenceJSON: evidenceMap,
	}
	if err := s.modRepo.CreateModerationAction(ctx, action); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    moderatorID,
		Action:     "moderation.report.resolve",
		EntityType: "report",
		EntityID:   reportID,
		AfterData:  map[string]interface{}{"action_type": actionType, "notes": notes},
		Source:     "moderation",
		Reason:     notes,
	})
	return nil
}

// DismissReport transitions a report to DISMISSED.
func (s *ModerationService) DismissReport(ctx context.Context, reportID, moderatorID uuid.UUID, notes string) error {
	report, err := s.modRepo.GetReport(ctx, reportID)
	if err != nil {
		return err
	}
	report.Status = "DISMISSED"
	report.Notes = notes
	if err := s.modRepo.UpdateReport(ctx, report); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    moderatorID,
		Action:     "moderation.report.dismiss",
		EntityType: "report",
		EntityID:   reportID,
		AfterData:  map[string]interface{}{"status": "DISMISSED", "notes": notes},
		Source:     "moderation",
		Reason:     notes,
	})
	return nil
}

// TakedownResource updates the resource status to TAKEN_DOWN, deducts points from the author, and records an action.
func (s *ModerationService) TakedownResource(ctx context.Context, resourceID, moderatorID uuid.UUID, evidence string) error {
	res, err := s.catRepo.GetResource(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("resource not found: %w", err)
	}

	// Update resource status directly
	res.Status = model.ResourceStatusTakenDown
	if err := s.catRepo.UpdateResource(ctx, res); err != nil {
		return fmt.Errorf("updating resource status: %w", err)
	}

	// Award TAKEDOWN_PENALTY points to the author (negative points via rule)
	srcID := resourceID
	_ = s.gamSvc.AwardPoints(ctx, res.AuthorID, string(model.PointEventTakedownPenalty), &srcID)

	// Record moderation action
	action := &model.ModerationAction{
		ID:          uuid.New(),
		ModeratorID: moderatorID,
		ActionType:  "TAKEDOWN",
		TargetType:  "RESOURCE",
		TargetID:    resourceID,
		Notes:       "Resource taken down",
		EvidenceJSON: map[string]interface{}{
			"evidence": evidence,
		},
	}
	if err := s.modRepo.CreateModerationAction(ctx, action); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    moderatorID,
		Action:     "moderation.resource.takedown",
		EntityType: "resource",
		EntityID:   resourceID,
		AfterData:  map[string]interface{}{"status": string(model.ResourceStatusTakenDown)},
		Source:     "moderation",
	})
	return nil
}

// RestoreResource restores a taken-down resource to PUBLISHED.
func (s *ModerationService) RestoreResource(ctx context.Context, resourceID, adminID uuid.UUID) error {
	res, err := s.catRepo.GetResource(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("resource not found: %w", err)
	}

	res.Status = model.ResourceStatusPublished
	if err := s.catRepo.UpdateResource(ctx, res); err != nil {
		return fmt.Errorf("restoring resource: %w", err)
	}

	action := &model.ModerationAction{
		ID:           uuid.New(),
		ModeratorID:  adminID,
		ActionType:   "RESTORE",
		TargetType:   "RESOURCE",
		TargetID:     resourceID,
		Notes:        "Resource restored",
		EvidenceJSON: map[string]interface{}{},
	}
	if err := s.modRepo.CreateModerationAction(ctx, action); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    adminID,
		Action:     "moderation.resource.restore",
		EntityType: "resource",
		EntityID:   resourceID,
		AfterData:  map[string]interface{}{"status": string(model.ResourceStatusPublished)},
		Source:     "moderation",
	})
	return nil
}

// BanUser creates a user ban. If PERMANENT, also sets users.status=BANNED.
func (s *ModerationService) BanUser(ctx context.Context, targetUserID, bannedByID uuid.UUID, banType, reason string) error {
	ban := &model.UserBan{
		ID:       uuid.New(),
		UserID:   targetUserID,
		BanType:  banType,
		Reason:   reason,
		BannedBy: bannedByID,
		IsActive: true,
	}

	switch banType {
	case "1_DAY":
		t := time.Now().Add(24 * time.Hour)
		ban.ExpiresAt = &t
	case "7_DAYS":
		t := time.Now().Add(7 * 24 * time.Hour)
		ban.ExpiresAt = &t
	case "PERMANENT":
		ban.ExpiresAt = nil
	default:
		return fmt.Errorf("%w: invalid ban type: %s", model.ErrValidation, banType)
	}

	if err := s.modRepo.CreateUserBan(ctx, ban); err != nil {
		return err
	}

	// For permanent bans, update user status to BANNED
	if banType == "PERMANENT" {
		if err := s.modRepo.SetUserStatus(ctx, targetUserID, "BANNED"); err != nil {
			return err
		}
	}

	// Record moderation action
	action := &model.ModerationAction{
		ID:          uuid.New(),
		ModeratorID: bannedByID,
		ActionType:  "BAN",
		TargetType:  "USER",
		TargetID:    targetUserID,
		Notes:       reason,
		EvidenceJSON: map[string]interface{}{
			"ban_type": banType,
		},
	}
	if err := s.modRepo.CreateModerationAction(ctx, action); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    bannedByID,
		Action:     "moderation.user.ban",
		EntityType: "user",
		EntityID:   targetUserID,
		AfterData:  map[string]interface{}{"ban_type": banType, "reason": reason},
		Source:     "moderation",
		Reason:     reason,
	})

	// Notify the banned user.
	if s.notifSvc != nil {
		banLabel := "temporarily"
		if banType == "PERMANENT" {
			banLabel = "permanently"
		}
		_ = s.notifSvc.Send(ctx, targetUserID, model.EventBanNotice,
			"Your account has been banned",
			fmt.Sprintf("Your account has been %s banned. Reason: %s", banLabel, reason),
			nil)
	}
	return nil
}

// UnbanUser deactivates a user's ban and restores their status to ACTIVE.
func (s *ModerationService) UnbanUser(ctx context.Context, targetUserID, adminID uuid.UUID) error {
	if err := s.modRepo.DeactivateBan(ctx, targetUserID); err != nil {
		return err
	}
	if err := s.modRepo.SetUserStatus(ctx, targetUserID, "ACTIVE"); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    adminID,
		Action:     "moderation.user.unban",
		EntityType: "user",
		EntityID:   targetUserID,
		AfterData:  map[string]interface{}{"status": "ACTIVE"},
		Source:     "moderation",
	})
	return nil
}

// ReviewAnomaly processes an anomaly flag as REVIEWED or DISMISSED.
func (s *ModerationService) ReviewAnomaly(ctx context.Context, flagID uuid.UUID, reviewerID uuid.UUID, decision string) error {
	if decision != "REVIEWED" && decision != "DISMISSED" {
		return fmt.Errorf("%w: invalid decision: %s", model.ErrValidation, decision)
	}

	// Get the flag to find user IDs if decision is REVIEWED
	flags, err := s.engRepo.ListAnomalyFlags(ctx, "")
	if err != nil {
		return err
	}

	var targetFlag *model.AnomalyFlag
	for i := range flags {
		if flags[i].ID == flagID {
			targetFlag = &flags[i]
			break
		}
	}

	if targetFlag == nil {
		return model.ErrNotFound
	}

	// If REVIEWED, delete votes from flagged users
	if decision == "REVIEWED" && len(targetFlag.UserIDs) > 0 {
		if err := s.modRepo.DeleteVotesByUsers(ctx, targetFlag.UserIDs); err != nil {
			return fmt.Errorf("deleting votes from ring members: %w", err)
		}
	}

	return s.engRepo.UpdateAnomalyFlag(ctx, flagID, decision)
}

// ListAnomalyFlags returns anomaly flags optionally filtered by status.
func (s *ModerationService) ListAnomalyFlags(ctx context.Context, status string) ([]model.AnomalyFlag, error) {
	return s.engRepo.ListAnomalyFlags(ctx, status)
}

// GetReport returns a single report with populated names.
func (s *ModerationService) GetReport(ctx context.Context, reportID uuid.UUID) (*model.Report, error) {
	return s.modRepo.GetReport(ctx, reportID)
}

// ListReports returns paginated reports optionally filtered by status.
func (s *ModerationService) ListReports(ctx context.Context, status string, page, pageSize int) ([]model.Report, int, error) {
	return s.modRepo.ListReports(ctx, status, page, pageSize)
}

// ListModerationActions returns moderation actions for a target.
func (s *ModerationService) ListModerationActions(ctx context.Context, targetType string, targetID uuid.UUID) ([]model.ModerationAction, error) {
	return s.modRepo.ListModerationActions(ctx, targetType, targetID)
}
