package activitycreator

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

const maxDurationMinutes = domain.MaxActivityDurationMinutes

type activityRepo interface {
	Create(ctx context.Context, a domain.Activity) (domain.Activity, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

// UseCase validates and stores a new activity.
type UseCase struct {
	l     logger.Logger
	repo  activityRepo
	goals goalRepo
}

// New creates a new activity creator use case.
func New(l logger.Logger, repo activityRepo, goals goalRepo) *UseCase {
	return &UseCase{
		l:     l,
		repo:  repo,
		goals: goals,
	}
}

// Execute validates business rules and persists the activity. Repeated goal ids are stored once;
// every goal id must be an active goal.
func (uc *UseCase) Execute(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	a.TitleUz = strings.TrimSpace(a.TitleUz)
	a.TitleRu = strings.TrimSpace(a.TitleRu)
	a.DescriptionUz = strings.TrimSpace(a.DescriptionUz)
	a.DescriptionRu = strings.TrimSpace(a.DescriptionRu)
	a.GoalIDs = domain.NormalizeIDs(a.GoalIDs)

	if err := validate(a); err != nil {
		return domain.Activity{}, err
	}

	n, err := uc.goals.CountActive(ctx, a.GoalIDs)
	if err != nil {
		return domain.Activity{}, err
	}

	if n != len(a.GoalIDs) {
		return domain.Activity{}, errs.ErrUnknownGoals
	}

	created, err := uc.repo.Create(ctx, a)
	if err != nil {
		return domain.Activity{}, err
	}

	logger.WithContext(uc.l, ctx).Info("activity created", zap.String("id", created.ID))

	return created, nil
}

func validate(a domain.Activity) error {
	switch {
	case a.TitleUz == "" || a.TitleRu == "":
		return errs.Errf(errs.ErrValidation, "title_uz and title_ru are required")
	case a.DescriptionUz == "" || a.DescriptionRu == "":
		return errs.Errf(errs.ErrValidation, "description_uz and description_ru are required")
	case len(a.GoalIDs) == 0 || len(a.GoalIDs) > domain.MaxActivityGoals:
		return errs.Errf(errs.ErrValidation, "goal_ids must hold 1 to %d goal ids", domain.MaxActivityGoals)
	case a.MinAge < domain.MinChildAge || a.MaxAge > domain.MaxChildAge || a.MinAge > a.MaxAge:
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case a.DurationMinutes <= 0 || a.DurationMinutes > maxDurationMinutes:
		return errs.Errf(errs.ErrValidation, "duration_minutes must be between 1 and %d", maxDurationMinutes)
	}

	for _, id := range a.GoalIDs {
		if uuid.Validate(id) != nil {
			return errs.Errf(errs.ErrValidation, "goal_ids must be goal ids (UUIDs), got %q", id)
		}
	}

	return nil
}
