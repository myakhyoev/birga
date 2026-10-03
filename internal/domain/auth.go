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
)

// IsKnown reports whether r is one of the user_role enum values.
func (r UserRole) IsKnown() bool {
	return r == UserRoleUser || r == UserRoleAdmin || r == UserRolePaidUser
}

// IsSelfAssignable reports whether a user may pick r at sign-up. admin is never self-assigned.
func (r UserRole) IsSelfAssignable() bool {
	return r == UserRoleUser || r == UserRolePaidUser
}

// SignUpRequest registers a user whose phone number passed a sign_up OTP check.
type SignUpRequest struct {
	Name        string
	Username    string
	Password    string
	PhoneNumber string
	// Role defaults to UserRoleUser when empty.
	Role UserRole
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
