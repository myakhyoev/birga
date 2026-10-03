package userdeleter

import (
	"context"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type userRepo interface {
	Delete(ctx context.Context, id string) error
}

// UseCase soft-deletes a user.
type UseCase struct {
	l    logger.Logger
	repo userRepo
}

// New creates a new user deleter use case.
func New(l logger.Logger, repo userRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute marks the user deleted. Their auth row (tokens) is removed by a
// database trigger, and their username and phone number become free to reuse.
func (uc *UseCase) Execute(ctx context.Context, id string) error {
	if err := uc.repo.Delete(ctx, id); err != nil {
		return err
	}

	logger.WithContext(uc.l, ctx).Info("user deleted", zap.String("id", id))

	return nil
}
