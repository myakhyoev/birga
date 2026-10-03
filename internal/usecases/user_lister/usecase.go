package userlister

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type userRepo interface {
	List(ctx context.Context, f domain.UserFilter) ([]domain.User, int, error)
}

// UseCase lists users with pagination.
type UseCase struct {
	l    logger.Logger
	repo userRepo
}

// New creates a new user lister use case.
func New(l logger.Logger, repo userRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute returns a page of active users and the total number of active users.
func (uc *UseCase) Execute(ctx context.Context, f domain.UserFilter) ([]domain.User, int, error) {
	return uc.repo.List(ctx, f)
}
