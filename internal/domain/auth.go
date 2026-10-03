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

// SignUpRequest registers a user whose phone number passed a sign_up OTP check.
type SignUpRequest struct {
	Name        string
	Username    string
	Password    string
	PhoneNumber string
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
	AccessTokenHash  string
	RefreshTokenHash string
}

// TokenClaims is what the API reads back from a valid token.
type TokenClaims struct {
	UserID string
	Type   TokenType
}

// HashToken returns the stored form of a token, so a database leak does not leak usable tokens.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
