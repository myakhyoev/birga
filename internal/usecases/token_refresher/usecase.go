package tokenrefresher

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type tokenIssuer interface {
	Issue(userID string, typ domain.TokenType) (string, error)
	Parse(token string, want domain.TokenType) (domain.TokenClaims, error)
	AccessTTL() time.Duration
}

type authRepo interface {
	Get(ctx context.Context, userID string) (domain.UserAuth, error)
	SetAccessToken(ctx context.Context, userID, tokenHash string) error
}

// UseCase trades a refresh token for a new access token.
type UseCase struct {
	l      logger.Logger
	tokens tokenIssuer
	auth   authRepo
}

// New creates a new token refresher use case.
func New(l logger.Logger, tokens tokenIssuer, auth authRepo) *UseCase {
	return &UseCase{l: l, tokens: tokens, auth: auth}
}

// Execute accepts a refresh token that is validly signed, not expired, and still the one stored
// in user_auth (so a deleted user's token stops working). The refresh token itself is unchanged.
func (uc *UseCase) Execute(ctx context.Context, refreshToken string) (domain.AccessToken, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return domain.AccessToken{}, errs.Errf(errs.ErrValidation, "refresh_token is required")
	}

	claims, err := uc.tokens.Parse(refreshToken, domain.TokenTypeRefresh)
	if err != nil {
		return domain.AccessToken{}, errs.ErrInvalidRefreshToken
	}

	a, err := uc.auth.Get(ctx, claims.UserID)
	if errors.Is(err, errs.ErrUserNotFound) {
		return domain.AccessToken{}, errs.ErrInvalidRefreshToken
	}

	if err != nil {
		return domain.AccessToken{}, err
	}

	if subtle.ConstantTimeCompare([]byte(a.RefreshTokenHash), []byte(domain.HashToken(refreshToken))) != 1 {
		return domain.AccessToken{}, errs.ErrInvalidRefreshToken
	}

	access, err := uc.tokens.Issue(claims.UserID, domain.TokenTypeAccess)
	if err != nil {
		return domain.AccessToken{}, err
	}

	if err := uc.auth.SetAccessToken(ctx, claims.UserID, domain.HashToken(access)); err != nil {
		return domain.AccessToken{}, err
	}

	return domain.AccessToken{Token: access, ExpiresIn: uc.tokens.AccessTTL()}, nil
}
