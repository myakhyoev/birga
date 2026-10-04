package userlogout

import (
	"context"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type authRepo interface {
	SetTokens(ctx context.Context, userID, accessTokenHash, refreshTokenHash string) error
}

// UseCase signs the user out.
type UseCase struct {
	l    logger.Logger
	auth authRepo
}

// New creates a new logout use case.
func New(l logger.Logger, auth authRepo) *UseCase {
	return &UseCase{l: l, auth: auth}
}

// Execute removes the user's stored token hashes, so their access and refresh tokens stop working
// at once. Only one token pair is valid at a time, so this signs the user out everywhere.
func (uc *UseCase) Execute(ctx context.Context, userID string) error {
	if err := uc.auth.SetTokens(ctx, userID, "", ""); err != nil {
		return err
	}

	logger.WithContext(uc.l, ctx).Info("user logged out", zap.String("user_id", userID))

	return nil
}
