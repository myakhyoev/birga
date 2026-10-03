package activitydeleter

import (
	"context"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type activityRepo interface {
	Delete(ctx context.Context, id string) error
}

// UseCase soft-deletes an activity.
type UseCase struct {
	l    logger.Logger
	repo activityRepo
}

// New creates a new activity deleter use case.
func New(l logger.Logger, repo activityRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute marks the activity deleted. It disappears from every read and from recommendations;
// completions that point at it are kept.
func (uc *UseCase) Execute(ctx context.Context, id string) error {
	if err := uc.repo.Delete(ctx, id); err != nil {
		return err
	}

	logger.WithContext(uc.l, ctx).Info("activity deleted", zap.String("id", id))

	return nil
}
