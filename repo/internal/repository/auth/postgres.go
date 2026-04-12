package authrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxFailedLogins  = 5
	lockoutDuration  = 15 * time.Minute
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

// New returns a UserRepository backed by PostgreSQL.
func New(pool *pgxpool.Pool) UserRepository {
	return &postgresRepo{pool: pool}
}

// ─── User ────────────────────────────────────────────────────────────────────

func (r *postgresRepo) Create(ctx context.Context, u *model.User, profile *model.UserProfile) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, status, version, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, 1, NOW(), NOW())`,
		u.ID, u.Username, u.Email, u.PasswordHash, u.Status.String(),
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO user_profiles (user_id, display_name, bio, avatar_path, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		u.ID, profile.DisplayName, profile.Bio, profile.AvatarPath,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *postgresRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	u := &model.User{}
	var statusStr string
	var lockedUntil *time.Time

	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, status, failed_login_count, locked_until, version, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &statusStr,
		&u.FailedLoginCount, &lockedUntil, &u.Version, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	u.LockedUntil = lockedUntil
	u.Status, err = model.ParseUserStatus(statusStr)
	if err != nil {
		return nil, fmt.Errorf("corrupt status in db: %w", err)
	}
	return u, nil
}

func (r *postgresRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	u := &model.User{}
	var statusStr string
	var lockedUntil *time.Time

	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, status, failed_login_count, locked_until, version, created_at, updated_at
		 FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &statusStr,
		&u.FailedLoginCount, &lockedUntil, &u.Version, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	u.LockedUntil = lockedUntil
	u.Status, err = model.ParseUserStatus(statusStr)
	if err != nil {
		return nil, fmt.Errorf("corrupt status in db: %w", err)
	}
	return u, nil
}

func (r *postgresRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	u := &model.User{}
	var statusStr string
	var lockedUntil *time.Time

	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, status, failed_login_count, locked_until, version, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &statusStr,
		&u.FailedLoginCount, &lockedUntil, &u.Version, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	u.LockedUntil = lockedUntil
	u.Status, err = model.ParseUserStatus(statusStr)
	if err != nil {
		return nil, fmt.Errorf("corrupt status in db: %w", err)
	}
	return u, nil
}

func (r *postgresRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.UserStatus, version int) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET status = $1, version = version + 1, updated_at = NOW()
		 WHERE id = $2 AND version = $3`,
		status.String(), id, version,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrStaleVersion
	}
	return nil
}

func (r *postgresRepo) UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		hash, id,
	)
	return err
}

// RecordFailedLogin increments the counter. If it reaches maxFailedLogins, sets locked_until.
// AUTH-02: no increment during an active lockout.
func (r *postgresRepo) RecordFailedLogin(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users
		 SET failed_login_count = CASE
		       WHEN locked_until IS NOT NULL AND locked_until > NOW() THEN failed_login_count
		       ELSE failed_login_count + 1
		     END,
		     locked_until = CASE
		       WHEN (failed_login_count + 1) >= $1 AND (locked_until IS NULL OR locked_until <= NOW())
		            THEN NOW() + $2::interval
		       WHEN locked_until IS NOT NULL AND locked_until > NOW() THEN locked_until
		       ELSE locked_until
		     END,
		     updated_at = NOW()
		 WHERE id = $3`,
		maxFailedLogins, lockoutDuration.String(), id,
	)
	return err
}

// ClearLockout resets failed_login_count and locked_until after a successful login (AUTH-03).
func (r *postgresRepo) ClearLockout(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET failed_login_count = 0, locked_until = NULL, updated_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}

func (r *postgresRepo) ListUsers(ctx context.Context, f ListFilter) ([]model.UserWithRoles, int, error) {
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	offset := (f.Page - 1) * f.PageSize

	// Build dynamic WHERE clause
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if f.Status != "" {
		where += fmt.Sprintf(" AND u.status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Search != "" {
		where += fmt.Sprintf(" AND (u.username ILIKE $%d OR u.email ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}
	if f.Role != "" {
		where += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id AND ur.role = $%d)", argIdx)
		args = append(args, f.Role)
		argIdx++
	}

	// Count query
	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(DISTINCT u.id) FROM users u %s`, where)
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query
	dataSQL := fmt.Sprintf(`
		SELECT u.id, u.username, u.email, u.status, u.failed_login_count, u.locked_until,
		       u.version, u.created_at, u.updated_at,
		       COALESCE(p.display_name,''), COALESCE(p.bio,''), COALESCE(p.avatar_path,'')
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id = u.id
		%s
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, f.PageSize, offset)

	rows, err := r.pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []model.UserWithRoles
	for rows.Next() {
		var uwr model.UserWithRoles
		var statusStr string
		var lockedUntil *time.Time

		err := rows.Scan(
			&uwr.User.ID, &uwr.User.Username, &uwr.User.Email, &statusStr,
			&uwr.User.FailedLoginCount, &lockedUntil, &uwr.User.Version,
			&uwr.User.CreatedAt, &uwr.User.UpdatedAt,
			&uwr.Profile.DisplayName, &uwr.Profile.Bio, &uwr.Profile.AvatarPath,
		)
		if err != nil {
			return nil, 0, err
		}
		uwr.User.LockedUntil = lockedUntil
		uwr.User.Status, _ = model.ParseUserStatus(statusStr)
		uwr.Profile.UserID = uwr.User.ID

		roles, err := r.GetRoles(ctx, uwr.User.ID)
		if err != nil {
			return nil, 0, err
		}
		uwr.Roles = roles
		results = append(results, uwr)
	}
	return results, total, rows.Err()
}

// ─── Roles ───────────────────────────────────────────────────────────────────

func (r *postgresRepo) GetRoles(ctx context.Context, userID uuid.UUID) ([]model.Role, error) {
	rows, err := r.pool.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []model.Role
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		role, err := model.ParseRole(s)
		if err != nil {
			continue // skip unknown roles
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *postgresRepo) AssignRole(ctx context.Context, userID uuid.UUID, role model.Role) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role, created_at) VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING`,
		userID, role.String(),
	)
	return err
}

func (r *postgresRepo) RemoveRole(ctx context.Context, userID uuid.UUID, role model.Role) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id = $1 AND role = $2`,
		userID, role.String(),
	)
	return err
}

// ─── Sessions ────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateSession(ctx context.Context, s *model.Session) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, token, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, NOW())`,
		s.ID, s.UserID, s.Token, s.ExpiresAt,
	)
	return err
}

func (r *postgresRepo) FindSession(ctx context.Context, token string) (*model.Session, error) {
	s := &model.Session{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token, expires_at, created_at FROM sessions WHERE token = $1 AND expires_at > NOW()`,
		token,
	).Scan(&s.ID, &s.UserID, &s.Token, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *postgresRepo) DeleteSession(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// ─── Profile ─────────────────────────────────────────────────────────────────

func (r *postgresRepo) GetProfile(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	p := &model.UserProfile{UserID: userID}
	err := r.pool.QueryRow(ctx,
		`SELECT display_name, bio, avatar_path, created_at, updated_at FROM user_profiles WHERE user_id = $1`,
		userID,
	).Scan(&p.DisplayName, &p.Bio, &p.AvatarPath, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return p, nil
}
