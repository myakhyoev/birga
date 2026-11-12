package streakgetter

import (
	"context"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

type planRepo interface {
	DoneDays(ctx context.Context, childID string) ([]time.Time, int, time.Time, error)
}

// UseCase summarises a child's streak. Only days of the child's plans count.
type UseCase struct {
	l        logger.Logger
	children childRepo
	plans    planRepo
	now      func() time.Time
}

// New creates a new streak getter use case.
func New(l logger.Logger, children childRepo, plans planRepo) *UseCase {
	return &UseCase{l: l, children: children, plans: plans, now: time.Now}
}

// Execute returns the child's streak as of today (Uzbekistan time).
func (uc *UseCase) Execute(ctx context.Context, userID, childID string) (domain.Streak, error) {
	if _, err := uc.children.GetForParent(ctx, childID, userID); err != nil {
		return domain.Streak{}, err
	}

	days, total, _, err := uc.plans.DoneDays(ctx, childID)
	if err != nil {
		return domain.Streak{}, err
	}

	return domain.ComputeStreak(days, domain.LocalDay(uc.now()), total), nil
}

// Days returns the child's streak calendar from `from` to `to` (dates, both inclusive): each day
// since the child's first plan started, up to today, done or missed. A zero from or to means the
// last 28 days up to today. More than domain.MaxStreakDays days, or from after to, is
// errs.ErrValidation.
func (uc *UseCase) Days(ctx context.Context, userID, childID string, from, to time.Time) ([]domain.StreakDay, error) {
	today := domain.LocalDay(uc.now())

	if to.IsZero() {
		to = today
	}

	if from.IsZero() {
		from = to.AddDate(0, 0, -27) // four weeks
	}

	if from.After(to) || to.Sub(from) >= domain.MaxStreakDays*24*time.Hour {
		return nil, errs.Errf(errs.ErrValidation, "from must not be after to, and the range must be at most %d days",
			domain.MaxStreakDays)
	}

	if _, err := uc.children.GetForParent(ctx, childID, userID); err != nil {
		return nil, err
	}

	done, _, first, err := uc.plans.DoneDays(ctx, childID)
	if err != nil {
		return nil, err
	}

	return domain.StreakDays(done, first, from, to, today), nil
}
