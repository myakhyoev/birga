package activitylister

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type activityRepo interface {
	List(ctx context.Context, f domain.ActivityFilter) ([]domain.Activity, int, error)
}

// UseCase lists activities with filtering and pagination.
type UseCase struct {
	l    logger.Logger
	repo activityRepo
}

// New creates a new activity lister use case.
func New(l logger.Logger, repo activityRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute returns a page of activities and the total number of matches.
func (uc *UseCase) Execute(ctx context.Context, f domain.ActivityFilter) ([]domain.Activity, int, error) {
	if f.GoalID != "" && uuid.Validate(f.GoalID) != nil {
		return nil, 0, errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID), got %q", f.GoalID)
	}

	if f.Age != 0 && (f.Age < domain.MinChildAge || f.Age > domain.MaxChildAge) {
		return nil, 0, errs.Errf(errs.ErrValidation, "age must be between %d and %d", domain.MinChildAge, domain.MaxChildAge)
	}

	return uc.repo.List(ctx, f)
}
