package userlogin

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type authRepo interface {
	GetByUsername(ctx context.Context, username string) (domain.UserAuth, error)
	SetTokens(ctx context.Context, userID, accessTokenHash, refreshTokenHash string) error
}

type tokenIssuer interface {
	Issue(userID string, role domain.UserRole, typ domain.TokenType) (string, error)
	AccessTTL() time.Duration
}

// dummyHash is compared against when the username is unknown, so an unknown username takes as
// long to answer as a wrong password and does not reveal which usernames exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("birga-dummy-password"), bcrypt.DefaultCost)

// UseCase signs a user in with their username and password.
type UseCase struct {
	l      logger.Logger
	auth   authRepo
	tokens tokenIssuer
}

// New creates a new login use case.
func New(l logger.Logger, auth authRepo, tokens tokenIssuer) *UseCase {
	return &UseCase{l: l, auth: auth, tokens: tokens}
}

// Execute checks the username (case-insensitive) and the bcrypt password, then stores and returns a
// new token pair. Only the newest pair is valid, so signing in signs out every other device.
// An unknown username, a user without a password, and a wrong password all give ErrInvalidCredentials.
func (uc *UseCase) Execute(ctx context.Context, username, password string) (domain.TokenPair, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || password == "" {
		return domain.TokenPair{}, errs.Errf(errs.ErrValidation, "username and password are required")
	}

	a, err := uc.auth.GetByUsername(ctx, username)
	if errors.Is(err, errs.ErrUserNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))

		return domain.TokenPair{}, errs.ErrInvalidCredentials
	}

	if err != nil {
		return domain.TokenPair{}, err
	}

	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return domain.TokenPair{}, errs.ErrInvalidCredentials
	}

	pair, err := uc.issue(a.UserID, a.Role)
	if err != nil {
		return domain.TokenPair{}, err
	}

	if err := uc.auth.SetTokens(ctx, a.UserID, domain.HashToken(pair.AccessToken), domain.HashToken(pair.RefreshToken)); err != nil {
		return domain.TokenPair{}, err
	}

	logger.WithContext(uc.l, ctx).Info("user logged in", zap.String("user_id", a.UserID))

	return pair, nil
}

func (uc *UseCase) issue(userID string, role domain.UserRole) (domain.TokenPair, error) {
	access, err := uc.tokens.Issue(userID, role, domain.TokenTypeAccess)
	if err != nil {
		return domain.TokenPair{}, err
	}

	refresh, err := uc.tokens.Issue(userID, role, domain.TokenTypeRefresh)
	if err != nil {
		return domain.TokenPair{}, err
	}

	return domain.TokenPair{AccessToken: access, RefreshToken: refresh, AccessExpiresIn: uc.tokens.AccessTTL()}, nil
}
