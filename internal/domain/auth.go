package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Password length limits, in bytes. bcrypt ignores everything after 72 bytes.
const (
	MinPasswordLength = 8
	MaxPasswordLength = 72
)

// TokenType tells an access token from a refresh token; each is accepted only where it belongs.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// UserRole says what a user may do. It is the user_auth.role enum.
type UserRole string

const (
	UserRoleUser     UserRole = "user"
	UserRoleAdmin    UserRole = "admin"
	UserRolePaidUser UserRole = "paid_user"
	// UserRoleUnverifiedUser is every new sign-up: the phone number has not been confirmed.
	UserRoleUnverifiedUser UserRole = "unverified_user"
)

// IsKnown reports whether r is one of the user_role enum values.
func (r UserRole) IsKnown() bool {
	return r == UserRoleUser || r == UserRoleAdmin || r == UserRolePaidUser || r == UserRoleUnverifiedUser
}

// SignUpRequest registers a user. The new user always gets UserRoleUnverifiedUser.
type SignUpRequest struct {
	Name        string
	Username    string
	Password    string
	PhoneNumber string
	// Relationship is required, one of the Relationship* values.
	Relationship string
	// GoalIDs are optional ids of active goals.
	GoalIDs []string
}

// TokenPair is what a client keeps after signing up. AccessExpiresIn is the access token lifetime.
type TokenPair struct {
	AccessToken     string
	RefreshToken    string
	AccessExpiresIn time.Duration
}

// AccessToken is a freshly issued access token and its lifetime.
type AccessToken struct {
	Token     string
	ExpiresIn time.Duration
}

// UserAuth is a user's row in user_auth. Tokens are stored as HashToken values.
type UserAuth struct {
	UserID           string
	Username         string
	PasswordHash     string
	Role             UserRole
	AccessTokenHash  string
	RefreshTokenHash string
}

// TokenClaims is what the API reads back from a valid token.
type TokenClaims struct {
	UserID string
	Type   TokenType
	Role   UserRole
}

// Principal is the signed-in user behind a request: who they are and their current role.
type Principal struct {
	UserID string
	Role   UserRole
}

// HashToken returns the stored form of a token, so a database leak does not leak usable tokens.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
