package goallister

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type goalRepo interface {
	List(ctx context.Context) ([]domain.Goal, error)
}

// UseCase lists the active goals.
type UseCase struct {
	l    logger.Logger
	repo goalRepo
}

// New creates a new goal lister use case.
func New(l logger.Logger, repo goalRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute returns every active goal, oldest first.
func (uc *UseCase) Execute(ctx context.Context) ([]domain.Goal, error) {
	return uc.repo.List(ctx)
}
