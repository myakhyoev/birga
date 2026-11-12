package todaylist

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

type planGetter interface {
	Execute(ctx context.Context, userID, childID string) (domain.Plan, error)
}

type activityRepo interface {
	Get(ctx context.Context, id string) (domain.Activity, error)
	Similar(ctx context.Context, f domain.SimilarFilter) ([]domain.Activity, error)
}

type extraRepo interface {
	ListDay(ctx context.Context, childID string, day time.Time) ([]domain.DayExtra, error)
	Add(ctx context.Context, childID string, day time.Time, activityID string) error
	Replace(ctx context.Context, childID string, day time.Time, oldID, newID string) error
	Pick(ctx context.Context, p domain.ExtraPick, day time.Time) (domain.Activity, error)
}

type planRepo interface {
	SetActivity(ctx context.Context, planID string, day int, activityID string) error
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

// ExtraRequest adds an activity to a child's day: ActivityID itself, or one picked for GoalID.
type ExtraRequest struct {
	UserID     string
	Pro        bool
	ChildID    string
	GoalID     string
	ActivityID string
}

// Similar limits.
const (
	DefaultSimilarLimit = 3
	MaxSimilarLimit     = 10
)

// UseCase builds a child's list for today: the plan activity and the activities a Pro parent
// added ("Bugungi reja", Figma 40-42).
type UseCase struct {
	l          logger.Logger
	children   childRepo
	plans      planGetter
	activities activityRepo
	extras     extraRepo
	goals      goalRepo
	planDays   planRepo
	now        func() time.Time
}

// New creates a new today list use case.
func New(l logger.Logger, children childRepo, plans planGetter, activities activityRepo, extras extraRepo, goals goalRepo,
	planDays planRepo,
) *UseCase {
	return &UseCase{
		l: l, children: children, plans: plans, activities: activities, extras: extras, goals: goals, planDays: planDays, now: time.Now,
	}
}

// Execute returns the child's list for today. Plan is the open plan day, or the plan day done
// today; nil when neither exists (the plan is done, or no activity fits the child's age). Only
// the child's parents may ask (errs.ErrChildNotFound otherwise).
func (uc *UseCase) Execute(ctx context.Context, userID, childID string) (domain.Today, error) {
	out, _, err := uc.load(ctx, userID, childID)

	return out, err
}

// load returns the child's list for today and the child's current plan.
func (uc *UseCase) load(ctx context.Context, userID, childID string) (domain.Today, domain.Plan, error) {
	if _, err := uc.children.GetForParent(ctx, childID, userID); err != nil {
		return domain.Today{}, domain.Plan{}, err
	}

	today := domain.LocalDay(uc.now())
	out := domain.Today{Day: today}

	plan, err := uc.currentPlan(ctx, userID, childID)
	if err != nil {
		return domain.Today{}, domain.Plan{}, err
	}

	for _, d := range plan.Days {
		doneToday := d.Status == domain.PlanDayDone && d.CompletedOn != nil && d.CompletedOn.Equal(today)
		if d.Status == domain.PlanDayToday || doneToday {
			out.Plan = &domain.TodayPlan{Day: d.Day, Status: d.Status, Activity: d.Activity}
			if doneToday {
				out.DoneCount++
			}

			break
		}
	}

	if out.Extras, err = uc.extras.ListDay(ctx, childID, today); err != nil {
		return domain.Today{}, domain.Plan{}, err
	}

	for _, e := range out.Extras {
		if e.Done {
			out.DoneCount++
		}
	}

	return out, plan, nil
}

// currentPlan returns the child's plan; a child with no plan (no activity fits) has no days.
func (uc *UseCase) currentPlan(ctx context.Context, userID, childID string) (domain.Plan, error) {
	plan, err := uc.plans.Execute(ctx, userID, childID)
	if errors.Is(err, errs.ErrNoRecommendation) {
		return domain.Plan{}, nil
	}

	return plan, err
}

// planActivityIDs returns the activities of all days of the plan.
func planActivityIDs(plan domain.Plan) []string {
	ids := make([]string, 0, len(plan.Days))
	for _, d := range plan.Days {
		ids = append(ids, d.Activity.ID)
	}

	return ids
}

// AddExtra adds an activity to the child's day and returns it. It needs Birga Pro
// (errs.ErrNeedsPro). With GoalID it picks a published activity for the child's age serving that
// goal, not in the child's plan and not yet in today's list (errs.ErrNoExtra when none is left);
// with ActivityID it adds that published activity, unless it is a plan day not done yet
// (errs.ErrActivityInPlan: done early it would have to be done again on its day). Adding an
// activity already in the list returns it unchanged; a day holds at most domain.MaxDayExtras added activities (errs.ErrTooManyExtras).
func (uc *UseCase) AddExtra(ctx context.Context, req ExtraRequest) (domain.DayExtra, error) {
	if !req.Pro {
		return domain.DayExtra{}, errs.ErrNeedsPro
	}

	switch {
	case (req.GoalID == "") == (req.ActivityID == ""):
		return domain.DayExtra{}, errs.Errf(errs.ErrValidation, "send either goal_id or activity_id")
	case req.GoalID != "" && uuid.Validate(req.GoalID) != nil:
		return domain.DayExtra{}, errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID), got %q", req.GoalID)
	case req.ActivityID != "" && uuid.Validate(req.ActivityID) != nil:
		return domain.DayExtra{}, errs.Errf(errs.ErrValidation, "activity_id must be an activity id (UUID), got %q", req.ActivityID)
	}

	child, err := uc.children.GetForParent(ctx, req.ChildID, req.UserID)
	if err != nil {
		return domain.DayExtra{}, err
	}

	cur, plan, err := uc.load(ctx, req.UserID, req.ChildID)
	if err != nil {
		return domain.DayExtra{}, err
	}

	var a domain.Activity

	if req.GoalID != "" {
		if a, err = uc.pick(ctx, child, cur, plan, req.GoalID); err != nil {
			return domain.DayExtra{}, err
		}
	} else {
		if a, err = uc.activities.Get(ctx, req.ActivityID); err != nil {
			return domain.DayExtra{}, err
		}

		if !a.IsPublished {
			return domain.DayExtra{}, errs.ErrActivityNotFound
		}

		if slices.ContainsFunc(plan.Days, func(d domain.PlanDay) bool {
			return d.Activity.ID == a.ID && d.Status != domain.PlanDayDone
		}) {
			return domain.DayExtra{}, errs.ErrActivityInPlan
		}
	}

	if i := slices.IndexFunc(cur.Extras, func(e domain.DayExtra) bool { return e.Activity.ID == a.ID }); i >= 0 {
		return cur.Extras[i], nil
	}

	if len(cur.Extras) >= domain.MaxDayExtras {
		return domain.DayExtra{}, errs.ErrTooManyExtras
	}

	if err := uc.extras.Add(ctx, child.ID, cur.Day, a.ID); err != nil {
		return domain.DayExtra{}, err
	}

	list, err := uc.extras.ListDay(ctx, child.ID, cur.Day)
	if err != nil {
		return domain.DayExtra{}, err
	}

	if i := slices.IndexFunc(list, func(e domain.DayExtra) bool { return e.Activity.ID == a.ID }); i >= 0 {
		return list[i], nil
	}

	return domain.DayExtra{Activity: a, AddedAt: uc.now()}, nil
}

func (uc *UseCase) pick(
	ctx context.Context, child domain.Child, cur domain.Today, plan domain.Plan, goalID string,
) (domain.Activity, error) {
	n, err := uc.goals.CountActive(ctx, []string{goalID})
	if err != nil {
		return domain.Activity{}, err
	}

	if n != 1 {
		return domain.Activity{}, errs.ErrUnknownGoals
	}

	p := domain.ExtraPick{ChildID: child.ID, Age: childAge(child), GoalIDs: []string{goalID}, ExcludeIDs: planActivityIDs(plan)}

	for _, e := range cur.Extras {
		p.ExcludeIDs = append(p.ExcludeIDs, e.Activity.ID)
	}

	return uc.extras.Pick(ctx, p, cur.Day)
}

// ChangeDifficulty swaps an item of the child's list for today ("Osonroq / Odatdagi / Qiyinroq")
// for another published activity of level req.Difficulty, for the child's age and the same goal:
// the plan day's goal for the open plan day, one of the activity's goals for an added activity. It
// is never the current activity, a day of the child's plan or already in today's list, so asking
// for the level the activity already has gives another one of that level. The new activity takes
// the item's place: on the plan day it counts toward the streak like the old one. It needs Birga
// Pro (errs.ErrNeedsPro); an activity not in today's list is errs.ErrNotInToday, one done today
// errs.ErrTodayItemDone, and no match errs.ErrNoDifficultyMatch.
func (uc *UseCase) ChangeDifficulty(ctx context.Context, req domain.DifficultyChange) (domain.Activity, error) {
	if !req.Pro {
		return domain.Activity{}, errs.ErrNeedsPro
	}

	switch {
	case uuid.Validate(req.ActivityID) != nil:
		return domain.Activity{}, errs.Errf(errs.ErrValidation, "activity_id must be an activity id (UUID), got %q", req.ActivityID)
	case !domain.ValidDifficulty(req.Difficulty):
		return domain.Activity{}, errs.Errf(errs.ErrValidation, "difficulty must be %s, %s or %s, got %q",
			domain.DifficultyEasy, domain.DifficultyNormal, domain.DifficultyDifficult, req.Difficulty)
	}

	child, err := uc.children.GetForParent(ctx, req.ChildID, req.UserID)
	if err != nil {
		return domain.Activity{}, err
	}

	cur, plan, err := uc.load(ctx, req.UserID, req.ChildID)
	if err != nil {
		return domain.Activity{}, err
	}

	var (
		goalIDs []string
		planDay int
	)

	switch i := slices.IndexFunc(cur.Extras, func(e domain.DayExtra) bool { return e.Activity.ID == req.ActivityID }); {
	case cur.Plan != nil && cur.Plan.Activity.ID == req.ActivityID:
		if cur.Plan.Status == domain.PlanDayDone {
			return domain.Activity{}, errs.ErrTodayItemDone
		}

		planDay, goalIDs = cur.Plan.Day, cur.Plan.Activity.GoalIDs
		if d := slices.IndexFunc(plan.Days, func(d domain.PlanDay) bool { return d.Day == planDay }); d >= 0 && plan.Days[d].GoalID != nil {
			goalIDs = []string{*plan.Days[d].GoalID}
		}
	case i >= 0:
		if cur.Extras[i].Done {
			return domain.Activity{}, errs.ErrTodayItemDone
		}

		goalIDs = cur.Extras[i].Activity.GoalIDs
	default:
		return domain.Activity{}, errs.ErrNotInToday
	}

	p := domain.ExtraPick{
		ChildID: child.ID, Age: childAge(child), GoalIDs: goalIDs, Difficulty: req.Difficulty, ExcludeIDs: planActivityIDs(plan),
	}
	for _, e := range cur.Extras {
		p.ExcludeIDs = append(p.ExcludeIDs, e.Activity.ID)
	}

	a, err := uc.extras.Pick(ctx, p, cur.Day)
	if errors.Is(err, errs.ErrNoExtra) {
		return domain.Activity{}, errs.ErrNoDifficultyMatch
	}

	if err != nil {
		return domain.Activity{}, err
	}

	if planDay > 0 {
		err = uc.planDays.SetActivity(ctx, plan.ID, planDay, a.ID)
	} else {
		err = uc.extras.Replace(ctx, child.ID, cur.Day, req.ActivityID, a.ID)
	}

	if err != nil {
		return domain.Activity{}, err
	}

	logger.WithContext(uc.l, ctx).Info("today activity difficulty changed", zap.String("child_id", child.ID),
		zap.String("from", req.ActivityID), zap.String("to", a.ID), zap.String("difficulty", req.Difficulty))

	return a, nil
}

// Similar returns up to limit published activities like activityID ("O‘xshash mashg‘ulotlar",
// Figma 42); it needs Birga Pro. With childID (one of the user's children) they fit the child's
// age, leave out the activities of the child's plan, and the ones the child never did come first;
// without it they overlap the activity's ages.
func (uc *UseCase) Similar(ctx context.Context, userID string, pro bool, activityID, childID string, limit int) ([]domain.Activity, error) {
	if !pro {
		return nil, errs.ErrNeedsPro
	}

	switch {
	case limit < 0 || limit > MaxSimilarLimit:
		return nil, errs.Errf(errs.ErrValidation, "limit must be between 1 and %d", MaxSimilarLimit)
	case limit == 0:
		limit = DefaultSimilarLimit
	}

	if uuid.Validate(activityID) != nil {
		return nil, errs.ErrActivityNotFound
	}

	a, err := uc.activities.Get(ctx, activityID)
	if err != nil {
		return nil, err
	}

	if !a.IsPublished {
		return nil, errs.ErrActivityNotFound
	}

	f := domain.SimilarFilter{ActivityID: a.ID, GoalIDs: a.GoalIDs, MinAge: a.MinAge, MaxAge: a.MaxAge, Limit: limit}

	if childID != "" {
		if uuid.Validate(childID) != nil {
			return nil, errs.Errf(errs.ErrValidation, "child_id must be a child id (UUID), got %q", childID)
		}

		child, err := uc.children.GetForParent(ctx, childID, userID)
		if err != nil {
			return nil, err
		}

		plan, err := uc.currentPlan(ctx, userID, child.ID)
		if err != nil {
			return nil, err
		}

		f.ChildID, f.Age, f.ExcludeIDs = child.ID, childAge(child), planActivityIDs(plan)
	}

	return uc.activities.Similar(ctx, f)
}

// childAge clamps the child's age to the activity range, as the plan does.
func childAge(c domain.Child) int {
	return min(max(c.AgeYears(), domain.MinChildAge), domain.MaxChildAge)
}
