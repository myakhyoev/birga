package plangetter

import (
	"context"
	"errors"
	"strconv"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

type planRepo interface {
	LockChild(ctx context.Context, childID string) error
	Current(ctx context.Context, childID string) (domain.Plan, []domain.PlanItem, []bool, error)
	Create(ctx context.Context, childID string, number int, startedOn time.Time, items []domain.PlanItem) (string, error)
	Pick(ctx context.Context, p domain.PlanPick) (domain.Activity, error)
	SetActivity(ctx context.Context, planID string, day int, activityID string) error
	DoneDays(ctx context.Context, childID string) ([]time.Time, int, time.Time, error)
}

type txRunner interface {
	InTx(ctx context.Context, h func(context.Context) error) error
}

// UseCase returns a child's current 7-day plan and starts the next one.
type UseCase struct {
	l        logger.Logger
	tx       txRunner
	children childRepo
	plans    planRepo
	now      func() time.Time
}

// New creates a new plan getter use case.
func New(l logger.Logger, tx txRunner, children childRepo, plans planRepo) *UseCase {
	return &UseCase{l: l, tx: tx, children: children, plans: plans, now: time.Now}
}

// Execute returns the child's current plan with each day's status, making the first plan when
// the child has none yet. A day not yet done whose activity was unpublished or deleted since gets
// a new activity. Only the child's parents may ask (errs.ErrChildNotFound otherwise); no activity
// for the child's age is errs.ErrNoRecommendation.
func (uc *UseCase) Execute(ctx context.Context, userID, childID string) (domain.Plan, error) {
	return uc.run(ctx, userID, childID, false)
}

// Next starts the child's next plan, today, from the child's goals as they are now. The current
// plan must be done (errs.ErrPlanNotDone otherwise); a child with no plan gets its first one.
func (uc *UseCase) Next(ctx context.Context, userID, childID string) (domain.Plan, error) {
	return uc.run(ctx, userID, childID, true)
}

// Start makes the first plan of a child that was just created, inside the caller's transaction.
// When no activity fits the child's age yet, it makes none; the first Execute tries again.
func (uc *UseCase) Start(ctx context.Context, child domain.Child) error {
	_, _, err := uc.create(ctx, child, 1)
	if errors.Is(err, errs.ErrNoRecommendation) {
		logger.WithContext(uc.l, ctx).Warn("no activity for the first plan", zap.String("child_id", child.ID))

		return nil
	}

	return err
}

func (uc *UseCase) run(ctx context.Context, userID, childID string, next bool) (domain.Plan, error) {
	child, err := uc.children.GetForParent(ctx, childID, userID)
	if err != nil {
		return domain.Plan{}, err
	}

	var plan domain.Plan

	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		if err := uc.plans.LockChild(ctx, child.ID); err != nil {
			return err
		}

		cur, items, usable, err := uc.plans.Current(ctx, child.ID)
		if err != nil {
			return err
		}

		doneToday, err := uc.doneToday(ctx, child.ID)
		if err != nil {
			return err
		}

		if cur.ID != "" && next {
			if _, completed := domain.BuildPlanDays(items, doneToday); !completed {
				return errs.ErrPlanNotDone
			}
		}

		if cur.ID == "" || next {
			if cur, items, err = uc.create(ctx, child, cur.Number+1); err != nil {
				return err
			}

			usable = nil
		}

		if items, err = uc.repair(ctx, child, cur, items, usable); err != nil {
			return err
		}

		plan = cur
		plan.Days, plan.Completed = domain.BuildPlanDays(items, doneToday)

		return nil
	})
	if err != nil {
		return domain.Plan{}, err
	}

	return plan, nil
}

// doneToday reports whether a plan day of the child was done today: then no other day opens today.
func (uc *UseCase) doneToday(ctx context.Context, childID string) (bool, error) {
	days, _, _, err := uc.plans.DoneDays(ctx, childID)
	if err != nil {
		return false, err
	}

	return len(days) > 0 && !days[0].Before(domain.LocalDay(uc.now())), nil
}

// create makes plan number n of the child, starting today: each day serves the next of the
// child's goals in turn, with a different activity while the catalogue allows.
func (uc *UseCase) create(ctx context.Context, child domain.Child, n int) (domain.Plan, []domain.PlanItem, error) {
	var (
		today = domain.LocalDay(uc.now())
		pick  = newPick(child, n)
		items = make([]domain.PlanItem, 0, domain.PlanLength)
	)

	for i, goalID := range domain.PlanGoals(child.GoalIDs, n) {
		it := domain.PlanItem{Day: i + 1, GoalID: goalID}

		a, err := uc.pick(ctx, pick, it)
		if err != nil {
			return domain.Plan{}, nil, err
		}

		it.Activity = a
		items = append(items, it)
		pick.ExcludeIDs = append(pick.ExcludeIDs, a.ID)
	}

	id, err := uc.plans.Create(ctx, child.ID, n, today, items)
	if err != nil {
		return domain.Plan{}, nil, err
	}

	logger.WithContext(uc.l, ctx).Info("plan started", zap.String("child_id", child.ID), zap.Int("number", n))

	return domain.Plan{ID: id, ChildID: child.ID, Number: n, StartedOn: today}, items, nil
}

// repair gives a day that is not done yet a new activity when its own was unpublished or deleted.
func (uc *UseCase) repair(
	ctx context.Context, child domain.Child, plan domain.Plan, items []domain.PlanItem, usable []bool,
) ([]domain.PlanItem, error) {
	pick := newPick(child, plan.Number)
	for _, it := range items {
		pick.ExcludeIDs = append(pick.ExcludeIDs, it.Activity.ID)
	}

	for i := range usable {
		if usable[i] || items[i].CompletedOn != nil {
			continue
		}

		a, err := uc.pick(ctx, pick, items[i])
		if err != nil {
			return nil, err
		}

		if err := uc.plans.SetActivity(ctx, plan.ID, items[i].Day, a.ID); err != nil {
			return nil, err
		}

		items[i].Activity = a
		pick.ExcludeIDs = append(pick.ExcludeIDs, a.ID)
	}

	return items, nil
}

func newPick(child domain.Child, n int) domain.PlanPick {
	return domain.PlanPick{
		ChildID:      child.ID,
		Age:          min(max(child.AgeYears(), domain.MinChildAge), domain.MaxChildAge),
		ChildGoalIDs: child.GoalIDs,
		ExcludeIDs:   make([]string, 0, domain.PlanLength),
		Seed:         strconv.Itoa(n),
	}
}

// pick chooses the activity for one day, repeating an activity already in the plan only when the
// catalogue for the child's age has nothing else.
func (uc *UseCase) pick(ctx context.Context, pick domain.PlanPick, it domain.PlanItem) (domain.Activity, error) {
	pick.GoalID = it.GoalID
	pick.Seed += "-" + strconv.Itoa(it.Day)

	a, err := uc.plans.Pick(ctx, pick)
	if errors.Is(err, errs.ErrNoRecommendation) && len(pick.ExcludeIDs) > 0 {
		pick.ExcludeIDs = nil
		a, err = uc.plans.Pick(ctx, pick)
	}

	return a, err
}
