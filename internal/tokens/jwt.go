// Package tokens issues and checks the API's JWTs (HS256).
package tokens

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// MinSecretLength is the shortest accepted JWT_SECRET, in bytes (the HS256 key size).
const MinSecretLength = 32

// claims: sub is the user id, typ says access or refresh, role is the user's role when the token
// was issued, jti makes every token unique.
type claims struct {
	Type domain.TokenType `json:"typ"`
	Role domain.UserRole  `json:"role"`
	jwt.RegisteredClaims
}

// Issuer signs and parses tokens.
type Issuer struct {
	cfg    config.JWTConfig
	secret []byte
	now    func() time.Time
}

// New returns an Issuer, or an error when the secret is too short.
func New(cfg config.JWTConfig) (*Issuer, error) {
	if len(cfg.Secret) < MinSecretLength {
		return nil, errs.Errf(errs.ErrInternal, "JWT_SECRET must be at least %d bytes", MinSecretLength)
	}

	return &Issuer{cfg: cfg, secret: []byte(cfg.Secret), now: time.Now}, nil
}

// AccessTTL is the access token lifetime.
func (i *Issuer) AccessTTL() time.Duration { return i.cfg.AccessTTL }

// Issue signs a token of type typ for userID with role.
func (i *Issuer) Issue(userID string, role domain.UserRole, typ domain.TokenType) (string, error) {
	ttl := i.cfg.AccessTTL
	if typ == domain.TokenTypeRefresh {
		ttl = i.cfg.RefreshTTL
	}

	now := i.now()

	c := claims{
		Type: typ,
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   i.cfg.Issuer,
			Subject:  userID,
			IssuedAt: jwt.NewNumericDate(now),
			ID:       uuid.NewString(),
		},
	}

	// A zero refresh TTL means the refresh token never expires (no exp claim). Access tokens always expire.
	if typ == domain.TokenTypeAccess || ttl > 0 {
		c.ExpiresAt = jwt.NewNumericDate(now.Add(ttl))
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.secret)
	if err != nil {
		return "", errs.Errf(errs.ErrInternal, "sign jwt: %s", err.Error())
	}

	return token, nil
}

// Parse checks the signature, issuer, expiry and type of token. Access tokens must carry exp;
// refresh tokens may omit it (JWT_REFRESH_TTL=0), but one that has exp is checked. Any failure is
// errs.ErrUnauthorized.
func (i *Issuer) Parse(token string, want domain.TokenType) (domain.TokenClaims, error) {
	var c claims

	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(i.cfg.Issuer),
		jwt.WithTimeFunc(i.now),
	}
	if want == domain.TokenTypeAccess {
		opts = append(opts, jwt.WithExpirationRequired())
	}

	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return i.secret, nil }, opts...)

	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return domain.TokenClaims{}, errs.Errf(errs.ErrUnauthorized, "token expired")
	case err != nil:
		return domain.TokenClaims{}, errs.Errf(errs.ErrUnauthorized, "invalid token")
	case c.Type != want:
		return domain.TokenClaims{}, errs.Errf(errs.ErrUnauthorized, "wrong token type, want %s", want)
	case uuid.Validate(c.Subject) != nil:
		return domain.TokenClaims{}, errs.Errf(errs.ErrUnauthorized, "invalid token subject")
	}

	return domain.TokenClaims{UserID: c.Subject, Type: c.Type, Role: c.Role}, nil
}
