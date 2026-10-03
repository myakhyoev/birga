package activityrecommender

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

type activityRepo interface {
	Recommend(ctx context.Context, q domain.RecommendationQuery) (domain.Activity, error)
}

// Request is what the caregiver asks for: the child and optionally a goal and the minutes they have.
type Request struct {
	UserID     string
	ChildID    string
	Goal       string
	MaxMinutes int
}

// UseCase picks today's activity for a child with simple rules.
type UseCase struct {
	l          logger.Logger
	children   childRepo
	activities activityRepo
	now        func() time.Time
}

// New creates a new activity recommender use case.
func New(l logger.Logger, children childRepo, activities activityRepo) *UseCase {
	return &UseCase{l: l, children: children, activities: activities, now: time.Now}
}

// Execute returns one published activity that fits the child's age (clamped to the 2..6 catalogue
// range), the goal and the time limit, preferring activities the child has not done yet.
func (uc *UseCase) Execute(ctx context.Context, req Request) (domain.Activity, error) {
	if req.Goal != "" && !domain.IsKnownGoal(req.Goal) {
		return domain.Activity{}, errs.Errf(errs.ErrValidation, "unknown goal %q", req.Goal)
	}

	if req.MaxMinutes < 0 {
		return domain.Activity{}, errs.Errf(errs.ErrValidation, "minutes must be positive")
	}

	child, err := uc.children.GetForParent(ctx, req.ChildID, req.UserID)
	if err != nil {
		return domain.Activity{}, err
	}

	return uc.activities.Recommend(ctx, domain.RecommendationQuery{
		ChildID:    child.ID,
		Age:        min(max(child.Age, domain.MinChildAge), domain.MaxChildAge),
		Goal:       req.Goal,
		MaxMinutes: req.MaxMinutes,
		Day:        domain.LocalDay(uc.now()),
	})
}
