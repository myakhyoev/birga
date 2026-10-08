package activityrecommender

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

type userRepo interface {
	Get(ctx context.Context, id string) (domain.User, error)
}

type activityRepo interface {
	Recommend(ctx context.Context, q domain.RecommendationQuery) (domain.Activity, error)
}

// Request is what the caregiver asks for: the child and optionally a goal (a goals table id) and the
// minutes they have.
type Request struct {
	UserID     string
	ChildID    string
	GoalID     string
	MaxMinutes int
}

// UseCase picks today's activity for a child with simple rules.
type UseCase struct {
	l          logger.Logger
	children   childRepo
	users      userRepo
	activities activityRepo
	now        func() time.Time
}

// New creates a new activity recommender use case.
func New(l logger.Logger, children childRepo, users userRepo, activities activityRepo) *UseCase {
	return &UseCase{l: l, children: children, users: users, activities: activities, now: time.Now}
}

// Execute returns one published activity that fits the child's age (clamped to the 2..6 catalogue
// range), the goal and the time limit. Without a goal, activities serving one of the goals the
// caregiver picked at sign-up come first. Within that, activities the child has not done yet win.
func (uc *UseCase) Execute(ctx context.Context, req Request) (domain.Activity, error) {
	if req.GoalID != "" && uuid.Validate(req.GoalID) != nil {
		return domain.Activity{}, errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID), got %q", req.GoalID)
	}

	if req.MaxMinutes < 0 {
		return domain.Activity{}, errs.Errf(errs.ErrValidation, "minutes must be positive")
	}

	child, err := uc.children.GetForParent(ctx, req.ChildID, req.UserID)
	if err != nil {
		return domain.Activity{}, err
	}

	var prefer []string

	if req.GoalID == "" {
		user, err := uc.users.Get(ctx, req.UserID)
		if err != nil {
			return domain.Activity{}, err
		}

		prefer = user.GoalIDs
	}

	return uc.activities.Recommend(ctx, domain.RecommendationQuery{
		ChildID:       child.ID,
		Age:           min(max(child.Age, domain.MinChildAge), domain.MaxChildAge),
		GoalID:        req.GoalID,
		PreferGoalIDs: prefer,
		MaxMinutes:    req.MaxMinutes,
		Day:           domain.LocalDay(uc.now()),
	})
}
