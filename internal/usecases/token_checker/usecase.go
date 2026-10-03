package tokenchecker

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type tokenParser interface {
	Parse(token string, want domain.TokenType) (domain.TokenClaims, error)
}

type authRepo interface {
	Get(ctx context.Context, userID string) (domain.UserAuth, error)
}

// UseCase authenticates an access token sent by the app.
type UseCase struct {
	l      logger.Logger
	tokens tokenParser
	auth   authRepo
}

// New creates a new token checker use case.
func New(l logger.Logger, tokens tokenParser, auth authRepo) *UseCase {
	return &UseCase{l: l, tokens: tokens, auth: auth}
}

// Execute returns the user id of a validly signed, unexpired access token that is still the one
// stored in user_auth: refreshing replaces it, and deleting the user removes it. Any failure is
// errs.ErrUnauthorized.
func (uc *UseCase) Execute(ctx context.Context, accessToken string) (string, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", errs.Errf(errs.ErrUnauthorized, "missing access token, send Authorization: Bearer <token>")
	}

	claims, err := uc.tokens.Parse(accessToken, domain.TokenTypeAccess)
	if err != nil {
		return "", err
	}

	a, err := uc.auth.Get(ctx, claims.UserID)
	if errors.Is(err, errs.ErrUserNotFound) {
		return "", errs.Errf(errs.ErrUnauthorized, "access token is no longer valid")
	}

	if err != nil {
		return "", err
	}

	if subtle.ConstantTimeCompare([]byte(a.AccessTokenHash), []byte(domain.HashToken(accessToken))) != 1 {
		return "", errs.Errf(errs.ErrUnauthorized, "access token is no longer valid, refresh it")
	}

	return claims.UserID, nil
}
