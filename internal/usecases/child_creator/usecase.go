package childcreator

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	Create(ctx context.Context, c domain.Child) (domain.Child, error)
	AddParent(ctx context.Context, childID, userID string) error
	List(ctx context.Context, f domain.ChildFilter) ([]domain.Child, int, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

type planStarter interface {
	Start(ctx context.Context, child domain.Child) error
}

type txRunner interface {
	InTx(ctx context.Context, h func(context.Context) error) error
}

// UseCase adds a child profile for the signed-in user.
type UseCase struct {
	l        logger.Logger
	tx       txRunner
	children childRepo
	goals    goalRepo
	plans    planStarter
}

// New creates a new child creator use case.
func New(l logger.Logger, tx txRunner, children childRepo, goals goalRepo, plans planStarter) *UseCase {
	return &UseCase{l: l, tx: tx, children: children, goals: goals, plans: plans}
}

// Execute validates the child, then in one transaction stores it in children, links it to userID
// in user_children, so a child never exists without a parent, and makes its first 7-day plan from
// its goals. The child needs at least one goal; repeated goal ids are stored once and every one
// must be an active goal. On the free plan (pro false) a user may have one child: adding another
// is errs.ErrNeedsPro.
func (uc *UseCase) Execute(ctx context.Context, userID string, pro bool, c domain.Child) (domain.Child, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Gender = strings.ToLower(strings.TrimSpace(c.Gender))

	if c.PhotoID != nil {
		if v := strings.TrimSpace(*c.PhotoID); v == "" {
			c.PhotoID = nil
		} else {
			c.PhotoID = &v
		}
	}

	c.GoalIDs = domain.NormalizeIDs(c.GoalIDs)

	if err := validate(c); err != nil {
		return domain.Child{}, err
	}

	if n, err := uc.goals.CountActive(ctx, c.GoalIDs); err != nil {
		return domain.Child{}, err
	} else if n != len(c.GoalIDs) {
		return domain.Child{}, errs.ErrUnknownGoals
	}

	if !pro {
		if _, n, err := uc.children.List(ctx, domain.ChildFilter{ParentID: userID, Limit: 1}); err != nil {
			return domain.Child{}, err
		} else if n >= domain.MaxFreeChildren {
			return domain.Child{}, errs.Errf(errs.ErrNeedsPro, "%s: the free plan has %d child", errs.ErrNeedsPro.Error(), domain.MaxFreeChildren)
		}
	}

	var created domain.Child

	err := uc.tx.InTx(ctx, func(ctx context.Context) error {
		var err error

		created, err = uc.children.Create(ctx, c)
		if err != nil {
			return err
		}

		if err := uc.children.AddParent(ctx, created.ID, userID); err != nil {
			return err
		}

		return uc.plans.Start(ctx, created)
	})
	if err != nil {
		return domain.Child{}, err
	}

	logger.WithContext(uc.l, ctx).Info("child created", zap.String("id", created.ID), zap.String("user_id", userID))

	return created, nil
}

func validate(c domain.Child) error {
	switch {
	case !domain.IsValidChildName(c.Name):
		return errs.Errf(errs.ErrValidation, "name is required, at most %d characters", domain.MaxChildNameLength)
	case !domain.IsValidChildAgeMonths(c.AgeMonths):
		return errs.Errf(errs.ErrValidation,
			"age_months must be 1 to 11, a half year from 12 to 84, or a whole year up to %d", domain.MaxChildAgeMonths)
	case !domain.IsKnownGender(c.Gender):
		return errs.Errf(errs.ErrValidation, "gender must be %s or %s", domain.GenderMale, domain.GenderFemale)
	case c.PhotoID != nil && uuid.Validate(*c.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a UUID")
	}

	return validateGoals(c.GoalIDs)
}

// validateGoals checks a child's goal ids: 1 to MaxChildGoals UUIDs.
func validateGoals(ids []string) error {
	if len(ids) == 0 || len(ids) > domain.MaxChildGoals {
		return errs.Errf(errs.ErrValidation, "goal_ids must hold 1 to %d goal ids from GET /v1/goals", domain.MaxChildGoals)
	}

	for _, id := range ids {
		if uuid.Validate(id) != nil {
			return errs.Errf(errs.ErrValidation, "goal_ids must be goal ids (UUIDs), got %q", id)
		}
	}

	return nil
}
