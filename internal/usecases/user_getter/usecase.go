package usergetter

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type userRepo interface {
	Get(ctx context.Context, id string) (domain.User, error)
}

// UseCase returns one user by id.
type UseCase struct {
	l    logger.Logger
	repo userRepo
}

// New creates a new user getter use case.
func New(l logger.Logger, repo userRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute returns the user; soft-deleted users are not found.
func (uc *UseCase) Execute(ctx context.Context, id string) (domain.User, error) {
	return uc.repo.Get(ctx, id)
}
