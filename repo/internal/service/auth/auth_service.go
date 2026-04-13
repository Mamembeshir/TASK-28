package authservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/eduexchange/eduexchange/internal/crypto"
	"github.com/eduexchange/eduexchange/internal/model"
	authrepo "github.com/eduexchange/eduexchange/internal/repository/auth"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 24 * time.Hour

// AuthService handles registration, login, logout and session validation.
//
// Password hashes are stored encrypted at rest (AES-256-GCM) using encryptionKey.
// When encryptionKey is nil or zero-length the service falls back to plain
// bcrypt storage so that unit tests that do not supply a key continue to work.
type AuthService struct {
	repo          authrepo.UserRepository
	encryptionKey []byte
}

func NewAuthService(repo authrepo.UserRepository, encryptionKey []byte) *AuthService {
	return &AuthService{repo: repo, encryptionKey: encryptionKey}
}

// encryptHash wraps a raw bcrypt hash with AES-256-GCM if a key is configured.
func (s *AuthService) encryptHash(hash string) (string, error) {
	if len(s.encryptionKey) == 0 {
		return hash, nil
	}
	return crypto.Encrypt(s.encryptionKey, []byte(hash))
}

// decryptHash undoes encryptHash.  If the stored value was never encrypted
// (e.g. written by a test without a key) it is returned as-is so that
// bcrypt.CompareHashAndPassword can still verify it.
func (s *AuthService) decryptHash(stored string) (string, error) {
	if len(s.encryptionKey) == 0 {
		return stored, nil
	}
	plain, err := crypto.Decrypt(s.encryptionKey, stored)
	if err != nil {
		// Value may be a legacy plain bcrypt hash — return it unchanged so
		// the caller can attempt a direct compare and handle the failure.
		return stored, nil
	}
	return string(plain), nil
}

// Register validates inputs, hashes the password, persists the user, and assigns REGULAR_USER.
// AUTH-05: self-registration always produces ACTIVE Regular User.
func (s *AuthService) Register(ctx context.Context, username, email, password string) (*model.User, error) {
	// Validate
	if errs := ValidateUsername(username); errs.HasErrors() {
		return nil, errs
	}
	if errs := ValidateEmail(email); errs.HasErrors() {
		return nil, errs
	}
	if errs := ValidatePassword(password); errs.HasErrors() {
		return nil, errs
	}

	// Uniqueness checks
	if _, err := s.repo.FindByUsername(ctx, username); err == nil {
		ve := model.NewValidationErrors()
		ve.Add("username", "Username is already taken.")
		return nil, ve
	}
	if _, err := s.repo.FindByEmail(ctx, email); err == nil {
		ve := model.NewValidationErrors()
		ve.Add("email", "Email address is already registered.")
		return nil, ve
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Encrypt the hash before persistence (at-rest compliance).
	encryptedHash, err := s.encryptHash(string(hash))
	if err != nil {
		return nil, err
	}

	u := &model.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        email,
		PasswordHash: encryptedHash,
		Status:       model.UserStatusActive,
	}
	profile := &model.UserProfile{
		UserID:      u.ID,
		DisplayName: username,
	}

	if err := s.repo.Create(ctx, u, profile); err != nil {
		return nil, err
	}

	if err := s.repo.AssignRole(ctx, u.ID, model.RoleRegularUser); err != nil {
		return nil, err
	}

	return u, nil
}

// LoginResult is returned on a successful login.
type LoginResult struct {
	User  *model.User
	Token string
}

// Login authenticates a user by username+password, enforces AUTH-02/03/04.
func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	u, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}

	// Auth-02: check lockout first
	if u.IsLocked() {
		return nil, model.ErrForbidden
	}

	// Auth-02: account must be active to log in
	if u.Status != model.UserStatusActive {
		return nil, model.ErrForbidden
	}

	// Decrypt the stored hash before comparing.
	hashForCompare, err := s.decryptHash(u.PasswordHash)
	if err != nil {
		return nil, err
	}

	// Compare password
	if err := bcrypt.CompareHashAndPassword([]byte(hashForCompare), []byte(password)); err != nil {
		// Record failure (AUTH-02)
		_ = s.repo.RecordFailedLogin(ctx, u.ID)
		return nil, model.ErrNotFound // return generic error to avoid username enumeration
	}

	// AUTH-03: clear lockout on success
	_ = s.repo.ClearLockout(ctx, u.ID)

	// AUTH-04: create session with 24h TTL
	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	session := &model.Session{
		ID:        uuid.New(),
		UserID:    u.ID,
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(sessionTTL),
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	return &LoginResult{User: u, Token: token}, nil
}

// Logout deletes the session token.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.repo.DeleteSession(ctx, token)
}

// GetCurrentUser resolves a session token to a fully populated UserWithRoles.
func (s *AuthService) GetCurrentUser(ctx context.Context, token string) (*model.UserWithRoles, error) {
	session, err := s.repo.FindSession(ctx, token)
	if err != nil {
		return nil, err
	}

	u, err := s.repo.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	profile, err := s.repo.GetProfile(ctx, u.ID)
	if err != nil {
		// Profile is optional — use empty struct if missing
		profile = &model.UserProfile{UserID: u.ID}
	}

	roles, err := s.repo.GetRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	return &model.UserWithRoles{User: *u, Profile: *profile, Roles: roles}, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
