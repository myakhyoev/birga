package streakgetter

import (
	"context"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

type completionRepo interface {
	Days(ctx context.Context, childID string) ([]time.Time, int, error)
}

// UseCase summarises a child's streak and progress.
type UseCase struct {
	l           logger.Logger
	children    childRepo
	completions completionRepo
	now         func() time.Time
}

// New creates a new streak getter use case.
func New(l logger.Logger, children childRepo, completions completionRepo) *UseCase {
	return &UseCase{l: l, children: children, completions: completions, now: time.Now}
}

// Execute returns the child's streak as of today (Uzbekistan time).
func (uc *UseCase) Execute(ctx context.Context, userID, childID string) (domain.Streak, error) {
	if _, err := uc.children.GetForParent(ctx, childID, userID); err != nil {
		return domain.Streak{}, err
	}

	days, total, err := uc.completions.Days(ctx, childID)
	if err != nil {
		return domain.Streak{}, err
	}

	return domain.ComputeStreak(days, domain.LocalDay(uc.now()), total), nil
}
