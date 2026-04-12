package authservice

import (
	"context"
	"errors"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	authrepo "github.com/eduexchange/eduexchange/internal/repository/auth"
	"github.com/google/uuid"
)

// UserService handles admin user management operations.
type UserService struct {
	repo      authrepo.UserRepository
	auditSvc  *audit.Service
}

func NewUserService(repo authrepo.UserRepository, auditSvc *audit.Service) *UserService {
	return &UserService{repo: repo, auditSvc: auditSvc}
}

// ListUsers returns a paginated, filtered list of users with their roles.
func (s *UserService) ListUsers(ctx context.Context, f authrepo.ListFilter) ([]model.UserWithRoles, int, error) {
	return s.repo.ListUsers(ctx, f)
}

// GetUser returns a fully populated UserWithRoles by ID.
func (s *UserService) GetUser(ctx context.Context, id uuid.UUID) (*model.UserWithRoles, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	profile, err := s.repo.GetProfile(ctx, id)
	if err != nil {
		profile = &model.UserProfile{UserID: id}
	}

	roles, err := s.repo.GetRoles(ctx, id)
	if err != nil {
		return nil, err
	}

	return &model.UserWithRoles{User: *u, Profile: *profile, Roles: roles}, nil
}

// TransitionStatus changes a user's status with optimistic locking.
// Validates the transition against PRD 9.1 state machine.
func (s *UserService) TransitionStatus(ctx context.Context, actorID, targetID uuid.UUID, to model.UserStatus, version int) error {
	u, err := s.repo.FindByID(ctx, targetID)
	if err != nil {
		return err
	}

	if err := ValidStatusTransition(u.Status, to); err != nil {
		return err
	}

	if err := s.repo.UpdateStatus(ctx, targetID, to, version); err != nil {
		return err
	}

	// Audit log
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    actorID,
		Action:     "user.status.transition",
		EntityType: "user",
		EntityID:   targetID,
		BeforeData: map[string]string{"status": u.Status.String()},
		AfterData:  map[string]string{"status": to.String()},
	})

	return nil
}

// AssignRole adds a role to a user (idempotent via ON CONFLICT DO NOTHING).
func (s *UserService) AssignRole(ctx context.Context, actorID, targetID uuid.UUID, role model.Role) error {
	if err := s.repo.AssignRole(ctx, targetID, role); err != nil {
		return err
	}

	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    actorID,
		Action:     "user.role.assign",
		EntityType: "user",
		EntityID:   targetID,
		AfterData:  map[string]string{"role": role.String()},
	})

	return nil
}

// RemoveRole removes a role from a user.
func (s *UserService) RemoveRole(ctx context.Context, actorID, targetID uuid.UUID, role model.Role) error {
	// Cannot remove the last role
	existing, err := s.repo.GetRoles(ctx, targetID)
	if err != nil {
		return err
	}

	hasRole := false
	for _, r := range existing {
		if r == role {
			hasRole = true
			break
		}
	}
	if !hasRole {
		return model.ErrNotFound
	}

	if len(existing) == 1 {
		ve := model.NewValidationErrors()
		ve.Add("role", "Cannot remove the user's last role.")
		return ve
	}

	if err := s.repo.RemoveRole(ctx, targetID, role); err != nil {
		return err
	}

	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    actorID,
		Action:     "user.role.remove",
		EntityType: "user",
		EntityID:   targetID,
		BeforeData: map[string]string{"role": role.String()},
	})

	return nil
}

// UnlockUser clears the failed login count and lockout for a user.
func (s *UserService) UnlockUser(ctx context.Context, actorID, targetID uuid.UUID) error {
	if err := s.repo.ClearLockout(ctx, targetID); err != nil {
		return err
	}

	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    actorID,
		Action:     "user.unlock",
		EntityType: "user",
		EntityID:   targetID,
	})

	return nil
}

// isValidationError checks if an error is a *model.ValidationErrors.
func isValidationError(err error) bool {
	var ve *model.ValidationErrors
	return errors.As(err, &ve)
}
