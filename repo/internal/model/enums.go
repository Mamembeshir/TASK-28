package model

import "fmt"

// UserStatus represents the status of a user account.
type UserStatus int

const (
	UserStatusActive UserStatus = iota
	UserStatusSuspended
	UserStatusBanned
	UserStatusDeactivated
)

func (s UserStatus) String() string {
	switch s {
	case UserStatusActive:
		return "ACTIVE"
	case UserStatusSuspended:
		return "SUSPENDED"
	case UserStatusBanned:
		return "BANNED"
	case UserStatusDeactivated:
		return "DEACTIVATED"
	default:
		return fmt.Sprintf("UserStatus(%d)", int(s))
	}
}

func ParseUserStatus(s string) (UserStatus, error) {
	switch s {
	case "ACTIVE":
		return UserStatusActive, nil
	case "SUSPENDED":
		return UserStatusSuspended, nil
	case "BANNED":
		return UserStatusBanned, nil
	case "DEACTIVATED":
		return UserStatusDeactivated, nil
	default:
		return 0, fmt.Errorf("invalid UserStatus: %q", s)
	}
}

// Role represents a user role.
type Role int

const (
	RoleRegularUser Role = iota
	RoleAuthor
	RoleReviewer
	RoleSupplier
	RoleAdmin
)

func (r Role) String() string {
	switch r {
	case RoleRegularUser:
		return "REGULAR_USER"
	case RoleAuthor:
		return "AUTHOR"
	case RoleReviewer:
		return "REVIEWER"
	case RoleSupplier:
		return "SUPPLIER"
	case RoleAdmin:
		return "ADMIN"
	default:
		return fmt.Sprintf("Role(%d)", int(r))
	}
}

func ParseRole(s string) (Role, error) {
	switch s {
	case "REGULAR_USER":
		return RoleRegularUser, nil
	case "AUTHOR":
		return RoleAuthor, nil
	case "REVIEWER":
		return RoleReviewer, nil
	case "SUPPLIER":
		return RoleSupplier, nil
	case "ADMIN":
		return RoleAdmin, nil
	default:
		return 0, fmt.Errorf("invalid Role: %q", s)
	}
}

// ResourceStatus represents the status of a learning resource.
type ResourceStatus int

const (
	ResourceStatusDraft ResourceStatus = iota
	ResourceStatusPendingReview
	ResourceStatusApproved
	ResourceStatusPublished
	ResourceStatusRejected
	ResourceStatusTakenDown
)

func (s ResourceStatus) String() string {
	switch s {
	case ResourceStatusDraft:
		return "DRAFT"
	case ResourceStatusPendingReview:
		return "PENDING_REVIEW"
	case ResourceStatusApproved:
		return "APPROVED"
	case ResourceStatusPublished:
		return "PUBLISHED"
	case ResourceStatusRejected:
		return "REJECTED"
	case ResourceStatusTakenDown:
		return "TAKEN_DOWN"
	default:
		return fmt.Sprintf("ResourceStatus(%d)", int(s))
	}
}

func ParseResourceStatus(s string) (ResourceStatus, error) {
	switch s {
	case "DRAFT":
		return ResourceStatusDraft, nil
	case "PENDING_REVIEW":
		return ResourceStatusPendingReview, nil
	case "APPROVED":
		return ResourceStatusApproved, nil
	case "PUBLISHED":
		return ResourceStatusPublished, nil
	case "REJECTED":
		return ResourceStatusRejected, nil
	case "TAKEN_DOWN":
		return ResourceStatusTakenDown, nil
	default:
		return 0, fmt.Errorf("invalid ResourceStatus: %q", s)
	}
}

// VoteType represents an upvote or downvote.
// String-typed so it maps directly to DB VARCHAR values "UP" / "DOWN".
type VoteType string

const (
	VoteTypeUp   VoteType = "UP"
	VoteTypeDown VoteType = "DOWN"
)

func (v VoteType) String() string { return string(v) }

func ParseVoteType(s string) (VoteType, error) {
	switch s {
	case "UP":
		return VoteTypeUp, nil
	case "DOWN":
		return VoteTypeDown, nil
	default:
		return "", fmt.Errorf("invalid VoteType: %q", s)
	}
}

// FollowTargetType represents what entity is being followed.
// String-typed so it maps directly to DB VARCHAR values "AUTHOR" / "TOPIC".
type FollowTargetType string

const (
	FollowTargetAuthor FollowTargetType = "AUTHOR"
	FollowTargetTopic  FollowTargetType = "TOPIC"
)

func ParseFollowTargetType(s string) (FollowTargetType, error) {
	switch s {
	case "AUTHOR":
		return FollowTargetAuthor, nil
	case "TOPIC":
		return FollowTargetTopic, nil
	default:
		return "", fmt.Errorf("invalid FollowTargetType: %q", s)
	}
}

// BanType represents the duration type of a ban.
type BanType int

const (
	BanType1Day BanType = iota
	BanType7Days
	BanTypePermanent
)

func (b BanType) String() string {
	switch b {
	case BanType1Day:
		return "1_DAY"
	case BanType7Days:
		return "7_DAYS"
	case BanTypePermanent:
		return "PERMANENT"
	default:
		return fmt.Sprintf("BanType(%d)", int(b))
	}
}

func ParseBanType(s string) (BanType, error) {
	switch s {
	case "1_DAY":
		return BanType1Day, nil
	case "7_DAYS":
		return BanType7Days, nil
	case "PERMANENT":
		return BanTypePermanent, nil
	default:
		return 0, fmt.Errorf("invalid BanType: %q", s)
	}
}

// SupplierTier represents the tier of a supplier.
type SupplierTier int

const (
	SupplierTierBronze SupplierTier = iota
	SupplierTierSilver
	SupplierTierGold
)

func (t SupplierTier) String() string {
	switch t {
	case SupplierTierBronze:
		return "BRONZE"
	case SupplierTierSilver:
		return "SILVER"
	case SupplierTierGold:
		return "GOLD"
	default:
		return fmt.Sprintf("SupplierTier(%d)", int(t))
	}
}

func ParseSupplierTier(s string) (SupplierTier, error) {
	switch s {
	case "BRONZE":
		return SupplierTierBronze, nil
	case "SILVER":
		return SupplierTierSilver, nil
	case "GOLD":
		return SupplierTierGold, nil
	default:
		return 0, fmt.Errorf("invalid SupplierTier: %q", s)
	}
}

// OrderStatus represents the status of a supplier order.
type OrderStatus int

const (
	OrderStatusCreated OrderStatus = iota
	OrderStatusConfirmed
	OrderStatusShipped
	OrderStatusReceived
	OrderStatusQCPassed
	OrderStatusQCFailed
	OrderStatusClosed
	OrderStatusCancelled
)

func (s OrderStatus) String() string {
	switch s {
	case OrderStatusCreated:
		return "CREATED"
	case OrderStatusConfirmed:
		return "CONFIRMED"
	case OrderStatusShipped:
		return "SHIPPED"
	case OrderStatusReceived:
		return "RECEIVED"
	case OrderStatusQCPassed:
		return "QC_PASSED"
	case OrderStatusQCFailed:
		return "QC_FAILED"
	case OrderStatusClosed:
		return "CLOSED"
	case OrderStatusCancelled:
		return "CANCELLED"
	default:
		return fmt.Sprintf("OrderStatus(%d)", int(s))
	}
}

func ParseOrderStatus(s string) (OrderStatus, error) {
	switch s {
	case "CREATED":
		return OrderStatusCreated, nil
	case "CONFIRMED":
		return OrderStatusConfirmed, nil
	case "SHIPPED":
		return OrderStatusShipped, nil
	case "RECEIVED":
		return OrderStatusReceived, nil
	case "QC_PASSED":
		return OrderStatusQCPassed, nil
	case "QC_FAILED":
		return OrderStatusQCFailed, nil
	case "CLOSED":
		return OrderStatusClosed, nil
	case "CANCELLED":
		return OrderStatusCancelled, nil
	default:
		return 0, fmt.Errorf("invalid OrderStatus: %q", s)
	}
}

// QCResultType represents the outcome of a quality control inspection.
type QCResultType int

const (
	QCResultPass QCResultType = iota
	QCResultFail
)

func (q QCResultType) String() string {
	switch q {
	case QCResultPass:
		return "PASS"
	case QCResultFail:
		return "FAIL"
	default:
		return fmt.Sprintf("QCResultType(%d)", int(q))
	}
}

func ParseQCResultType(s string) (QCResultType, error) {
	switch s {
	case "PASS":
		return QCResultPass, nil
	case "FAIL":
		return QCResultFail, nil
	default:
		return 0, fmt.Errorf("invalid QCResultType: %q", s)
	}
}

// SupplierStatus represents the status of a supplier.
type SupplierStatus int

const (
	SupplierStatusActive SupplierStatus = iota
	SupplierStatusSuspended
)

func (s SupplierStatus) String() string {
	switch s {
	case SupplierStatusActive:
		return "ACTIVE"
	case SupplierStatusSuspended:
		return "SUSPENDED"
	default:
		return fmt.Sprintf("SupplierStatus(%d)", int(s))
	}
}

func ParseSupplierStatus(s string) (SupplierStatus, error) {
	switch s {
	case "ACTIVE":
		return SupplierStatusActive, nil
	case "SUSPENDED":
		return SupplierStatusSuspended, nil
	default:
		return 0, fmt.Errorf("invalid SupplierStatus: %q", s)
	}
}

// ReportStatus represents the status of a moderation report.
type ReportStatus int

const (
	ReportStatusOpen ReportStatus = iota
	ReportStatusUnderReview
	ReportStatusResolved
	ReportStatusDismissed
)

func (s ReportStatus) String() string {
	switch s {
	case ReportStatusOpen:
		return "OPEN"
	case ReportStatusUnderReview:
		return "UNDER_REVIEW"
	case ReportStatusResolved:
		return "RESOLVED"
	case ReportStatusDismissed:
		return "DISMISSED"
	default:
		return fmt.Sprintf("ReportStatus(%d)", int(s))
	}
}

func ParseReportStatus(s string) (ReportStatus, error) {
	switch s {
	case "OPEN":
		return ReportStatusOpen, nil
	case "UNDER_REVIEW":
		return ReportStatusUnderReview, nil
	case "RESOLVED":
		return ReportStatusResolved, nil
	case "DISMISSED":
		return ReportStatusDismissed, nil
	default:
		return 0, fmt.Errorf("invalid ReportStatus: %q", s)
	}
}

// ReportReasonType represents the reason for a moderation report.
type ReportReasonType int

const (
	ReportReasonSpam ReportReasonType = iota
	ReportReasonInappropriate
	ReportReasonCopyright
	ReportReasonOther
)

func (r ReportReasonType) String() string {
	switch r {
	case ReportReasonSpam:
		return "SPAM"
	case ReportReasonInappropriate:
		return "INAPPROPRIATE"
	case ReportReasonCopyright:
		return "COPYRIGHT"
	case ReportReasonOther:
		return "OTHER"
	default:
		return fmt.Sprintf("ReportReasonType(%d)", int(r))
	}
}

func ParseReportReasonType(s string) (ReportReasonType, error) {
	switch s {
	case "SPAM":
		return ReportReasonSpam, nil
	case "INAPPROPRIATE":
		return ReportReasonInappropriate, nil
	case "COPYRIGHT":
		return ReportReasonCopyright, nil
	case "OTHER":
		return ReportReasonOther, nil
	default:
		return 0, fmt.Errorf("invalid ReportReasonType: %q", s)
	}
}

// ModerationActionType represents the type of moderation action taken.
type ModerationActionType int

const (
	ModerationActionApprove ModerationActionType = iota
	ModerationActionReject
	ModerationActionTakedown
	ModerationActionRestore
	ModerationActionWarn
	ModerationActionBan
)

func (a ModerationActionType) String() string {
	switch a {
	case ModerationActionApprove:
		return "APPROVE"
	case ModerationActionReject:
		return "REJECT"
	case ModerationActionTakedown:
		return "TAKEDOWN"
	case ModerationActionRestore:
		return "RESTORE"
	case ModerationActionWarn:
		return "WARN"
	case ModerationActionBan:
		return "BAN"
	default:
		return fmt.Sprintf("ModerationActionType(%d)", int(a))
	}
}

func ParseModerationActionType(s string) (ModerationActionType, error) {
	switch s {
	case "APPROVE":
		return ModerationActionApprove, nil
	case "REJECT":
		return ModerationActionReject, nil
	case "TAKEDOWN":
		return ModerationActionTakedown, nil
	case "RESTORE":
		return ModerationActionRestore, nil
	case "WARN":
		return ModerationActionWarn, nil
	case "BAN":
		return ModerationActionBan, nil
	default:
		return 0, fmt.Errorf("invalid ModerationActionType: %q", s)
	}
}

// AnomalyFlagType represents the type of anomaly detected.
type AnomalyFlagType int

const (
	AnomalyFlagLikeRing AnomalyFlagType = iota
	AnomalyFlagRateSpike
	AnomalyFlagOther
)

func (a AnomalyFlagType) String() string {
	switch a {
	case AnomalyFlagLikeRing:
		return "LIKE_RING"
	case AnomalyFlagRateSpike:
		return "RATE_SPIKE"
	case AnomalyFlagOther:
		return "OTHER"
	default:
		return fmt.Sprintf("AnomalyFlagType(%d)", int(a))
	}
}

func ParseAnomalyFlagType(s string) (AnomalyFlagType, error) {
	switch s {
	case "LIKE_RING":
		return AnomalyFlagLikeRing, nil
	case "RATE_SPIKE":
		return AnomalyFlagRateSpike, nil
	case "OTHER":
		return AnomalyFlagOther, nil
	default:
		return 0, fmt.Errorf("invalid AnomalyFlagType: %q", s)
	}
}

// AnomalyFlagStatus represents the review status of an anomaly flag.
type AnomalyFlagStatus int

const (
	AnomalyFlagStatusOpen AnomalyFlagStatus = iota
	AnomalyFlagStatusReviewed
	AnomalyFlagStatusDismissed
)

func (s AnomalyFlagStatus) String() string {
	switch s {
	case AnomalyFlagStatusOpen:
		return "OPEN"
	case AnomalyFlagStatusReviewed:
		return "REVIEWED"
	case AnomalyFlagStatusDismissed:
		return "DISMISSED"
	default:
		return fmt.Sprintf("AnomalyFlagStatus(%d)", int(s))
	}
}

func ParseAnomalyFlagStatus(s string) (AnomalyFlagStatus, error) {
	switch s {
	case "OPEN":
		return AnomalyFlagStatusOpen, nil
	case "REVIEWED":
		return AnomalyFlagStatusReviewed, nil
	case "DISMISSED":
		return AnomalyFlagStatusDismissed, nil
	default:
		return 0, fmt.Errorf("invalid AnomalyFlagStatus: %q", s)
	}
}

// NotificationRetryStatus represents the delivery status of a notification.
type NotificationRetryStatus int

const (
	NotificationRetryPending NotificationRetryStatus = iota
	NotificationRetryDelivered
	NotificationRetryFailed
)

func (s NotificationRetryStatus) String() string {
	switch s {
	case NotificationRetryPending:
		return "PENDING"
	case NotificationRetryDelivered:
		return "DELIVERED"
	case NotificationRetryFailed:
		return "FAILED"
	default:
		return fmt.Sprintf("NotificationRetryStatus(%d)", int(s))
	}
}

func ParseNotificationRetryStatus(s string) (NotificationRetryStatus, error) {
	switch s {
	case "PENDING":
		return NotificationRetryPending, nil
	case "DELIVERED":
		return NotificationRetryDelivered, nil
	case "FAILED":
		return NotificationRetryFailed, nil
	default:
		return 0, fmt.Errorf("invalid NotificationRetryStatus: %q", s)
	}
}

// BulkImportStatus represents the status of a bulk import job.
type BulkImportStatus int

const (
	BulkImportStatusPending BulkImportStatus = iota
	BulkImportStatusProcessing
	BulkImportStatusPreviewReady
	BulkImportStatusConfirmed
	BulkImportStatusFailed
)

func (s BulkImportStatus) String() string {
	switch s {
	case BulkImportStatusPending:
		return "PENDING"
	case BulkImportStatusProcessing:
		return "PROCESSING"
	case BulkImportStatusPreviewReady:
		return "PREVIEW_READY"
	case BulkImportStatusConfirmed:
		return "CONFIRMED"
	case BulkImportStatusFailed:
		return "FAILED"
	default:
		return fmt.Sprintf("BulkImportStatus(%d)", int(s))
	}
}

func ParseBulkImportStatus(s string) (BulkImportStatus, error) {
	switch s {
	case "PENDING":
		return BulkImportStatusPending, nil
	case "PROCESSING":
		return BulkImportStatusProcessing, nil
	case "PREVIEW_READY":
		return BulkImportStatusPreviewReady, nil
	case "CONFIRMED":
		return BulkImportStatusConfirmed, nil
	case "FAILED":
		return BulkImportStatusFailed, nil
	default:
		return 0, fmt.Errorf("invalid BulkImportStatus: %q", s)
	}
}

// ScheduledReportStatus represents the status of a scheduled report.
type ScheduledReportStatus int

const (
	ScheduledReportStatusPending ScheduledReportStatus = iota
	ScheduledReportStatusGenerated
	ScheduledReportStatusFailed
)

func (s ScheduledReportStatus) String() string {
	switch s {
	case ScheduledReportStatusPending:
		return "PENDING"
	case ScheduledReportStatusGenerated:
		return "GENERATED"
	case ScheduledReportStatusFailed:
		return "FAILED"
	default:
		return fmt.Sprintf("ScheduledReportStatus(%d)", int(s))
	}
}

func ParseScheduledReportStatus(s string) (ScheduledReportStatus, error) {
	switch s {
	case "PENDING":
		return ScheduledReportStatusPending, nil
	case "GENERATED":
		return ScheduledReportStatusGenerated, nil
	case "FAILED":
		return ScheduledReportStatusFailed, nil
	default:
		return 0, fmt.Errorf("invalid ScheduledReportStatus: %q", s)
	}
}

// EventType represents a notification event type.
type EventType string

const (
	EventEntryDeadline    EventType = "entry_deadline"
	EventReviewDecision   EventType = "review_decision"
	EventPublishComplete  EventType = "publish_complete"
	EventSupplierShipment EventType = "supplier_shipment"
	EventSupplierQC       EventType = "supplier_qc"
	EventAnomalyAlert     EventType = "anomaly_alert"
	EventBanNotice        EventType = "ban_notice"
	EventReportUpdate     EventType = "report_update"
	EventBadgeEarned      EventType = "badge_earned"
	EventLevelUp          EventType = "level_up"
	EventFollowNewContent EventType = "follow_new_content"
)

// PointEventType represents the type of event that awards points.
type PointEventType string

const (
	PointEventEntryApproved   PointEventType = "ENTRY_APPROVED"
	PointEventUpvoteReceived  PointEventType = "UPVOTE_RECEIVED"
	PointEventDownvoteReceived PointEventType = "DOWNVOTE_RECEIVED"
	PointEventFavoriteReceived PointEventType = "FAVORITE_RECEIVED"
	PointEventTakedownPenalty PointEventType = "TAKEDOWN_PENALTY"
)
