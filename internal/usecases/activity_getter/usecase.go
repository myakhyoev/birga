package activitygetter

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type activityRepo interface {
	Get(ctx context.Context, id string) (domain.Activity, error)
}

// UseCase returns one activity by id.
type UseCase struct {
	l    logger.Logger
	repo activityRepo
}

// New creates a new activity getter use case.
func New(l logger.Logger, repo activityRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute returns the activity. Unpublished activities are hidden unless
// includeUnpublished is set (admin callers).
func (uc *UseCase) Execute(ctx context.Context, id string, includeUnpublished bool) (domain.Activity, error) {
	a, err := uc.repo.Get(ctx, id)
	if err != nil {
		return domain.Activity{}, err
	}

	if !a.IsPublished && !includeUnpublished {
		return domain.Activity{}, errs.ErrActivityNotFound
	}

	return a, nil
}
