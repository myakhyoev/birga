package goaldeleter

import (
	"context"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type goalRepo interface {
	Delete(ctx context.Context, id string) error
}

// UseCase soft-deletes a goal.
type UseCase struct {
	l    logger.Logger
	repo goalRepo
}

// New creates a new goal deleter use case.
func New(l logger.Logger, repo goalRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute marks the goal deleted and removes it from every user's goals. Its names become free again.
func (uc *UseCase) Execute(ctx context.Context, id string) error {
	if err := uc.repo.Delete(ctx, id); err != nil {
		return err
	}

	logger.WithContext(uc.l, ctx).Info("goal deleted", zap.String("id", id))

	return nil
}
