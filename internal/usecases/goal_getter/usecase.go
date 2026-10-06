package goalgetter

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type goalRepo interface {
	Get(ctx context.Context, id string) (domain.Goal, error)
}

// UseCase returns one active goal by id.
type UseCase struct {
	l    logger.Logger
	repo goalRepo
}

// New creates a new goal getter use case.
func New(l logger.Logger, repo goalRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute returns the goal; a missing or deleted goal is ErrGoalNotFound.
func (uc *UseCase) Execute(ctx context.Context, id string) (domain.Goal, error) {
	return uc.repo.Get(ctx, id)
}
