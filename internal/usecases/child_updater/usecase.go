package childupdater

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
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
	Update(ctx context.Context, id string, upd domain.ChildUpdate) (domain.Child, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

// UseCase lets a parent edit one of their children.
type UseCase struct {
	l        logger.Logger
	children childRepo
	goals    goalRepo
}

// New creates a new child updater use case.
func New(l logger.Logger, children childRepo, goals goalRepo) *UseCase {
	return &UseCase{l: l, children: children, goals: goals}
}

// Execute applies upd to the child if userID is one of its parents; any other child is
// errs.ErrChildNotFound. The fields follow the add-child rules; PhotoID "" clears the photo.
// GoalIDs replaces the child's goals; the current plan keeps its days, the next plan uses them.
func (uc *UseCase) Execute(ctx context.Context, userID, childID string, upd domain.ChildUpdate) (domain.Child, error) {
	if upd.IsEmpty() {
		return domain.Child{}, errs.Errf(errs.ErrValidation, "nothing to update")
	}

	if upd.Name != nil {
		v := strings.TrimSpace(*upd.Name)
		upd.Name = &v
	}

	if upd.Gender != nil {
		v := strings.ToLower(strings.TrimSpace(*upd.Gender))
		upd.Gender = &v
	}

	if upd.PhotoID != nil {
		v := strings.TrimSpace(*upd.PhotoID)
		upd.PhotoID = &v
	}

	if upd.GoalIDs != nil {
		ids := domain.NormalizeIDs(*upd.GoalIDs)
		upd.GoalIDs = &ids
	}

	if err := validate(upd); err != nil {
		return domain.Child{}, err
	}

	if upd.GoalIDs != nil {
		if n, err := uc.goals.CountActive(ctx, *upd.GoalIDs); err != nil {
			return domain.Child{}, err
		} else if n != len(*upd.GoalIDs) {
			return domain.Child{}, errs.ErrUnknownGoals
		}
	}

	if _, err := uc.children.GetForParent(ctx, childID, userID); err != nil {
		return domain.Child{}, err
	}

	updated, err := uc.children.Update(ctx, childID, upd)
	if err != nil {
		return domain.Child{}, err
	}

	logger.WithContext(uc.l, ctx).Info("child updated", zap.String("id", childID), zap.String("user_id", userID))

	return updated, nil
}

func validate(upd domain.ChildUpdate) error {
	switch {
	case upd.Name != nil && !domain.IsValidChildName(*upd.Name):
		return errs.Errf(errs.ErrValidation, "name is required, at most %d characters", domain.MaxChildNameLength)
	case upd.AgeMonths != nil && !domain.IsValidChildAgeMonths(*upd.AgeMonths):
		return errs.Errf(errs.ErrValidation,
			"age_months must be 1 to 11, a half year from 12 to 84, or a whole year up to %d", domain.MaxChildAgeMonths)
	case upd.Gender != nil && !domain.IsKnownGender(*upd.Gender):
		return errs.Errf(errs.ErrValidation, "gender must be %s or %s", domain.GenderMale, domain.GenderFemale)
	case upd.PhotoID != nil && *upd.PhotoID != "" && uuid.Validate(*upd.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a UUID")
	case upd.GoalIDs != nil:
		return validateGoals(*upd.GoalIDs)
	}

	return nil
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
