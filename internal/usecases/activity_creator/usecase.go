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

var errDifficulty = errs.Errf(errs.ErrValidation, "difficulty must be %s, %s or %s",
	domain.DifficultyEasy, domain.DifficultyNormal, domain.DifficultyDifficult)

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
	a.TitleEn = strings.TrimSpace(a.TitleEn)
	a.DescriptionUz = strings.TrimSpace(a.DescriptionUz)
	a.DescriptionRu = strings.TrimSpace(a.DescriptionRu)
	a.DescriptionEn = strings.TrimSpace(a.DescriptionEn)
	a.MainIdeaUz = strings.TrimSpace(a.MainIdeaUz)
	a.MainIdeaRu = strings.TrimSpace(a.MainIdeaRu)
	a.MainIdeaEn = strings.TrimSpace(a.MainIdeaEn)
	for _, s := range []*string{&a.EasierUz, &a.EasierRu, &a.EasierEn, &a.HarderUz, &a.HarderRu, &a.HarderEn} {
		*s = strings.TrimSpace(*s)
	}
	a.GoalIDs = domain.NormalizeIDs(a.GoalIDs)
	a.Difficulty = strings.ToLower(strings.TrimSpace(a.Difficulty))
	if a.Difficulty == "" {
		a.Difficulty = domain.DifficultyNormal
	}

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
	case a.TitleUz == "" || a.TitleRu == "" || a.TitleEn == "":
		return errs.Errf(errs.ErrValidation, "title_uz, title_ru and title_en are required")
	case a.DescriptionUz == "" || a.DescriptionRu == "" || a.DescriptionEn == "":
		return errs.Errf(errs.ErrValidation, "description_uz, description_ru and description_en are required")
	case len(a.GoalIDs) == 0 || len(a.GoalIDs) > domain.MaxActivityGoals:
		return errs.Errf(errs.ErrValidation, "goal_ids must hold 1 to %d goal ids", domain.MaxActivityGoals)
	case a.MinAge < domain.MinChildAge || a.MaxAge > domain.MaxChildAge || a.MinAge > a.MaxAge:
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case a.DurationMinutes <= 0 || a.DurationMinutes > maxDurationMinutes:
		return errs.Errf(errs.ErrValidation, "duration_minutes must be between 1 and %d", maxDurationMinutes)
	case !domain.ValidDifficulty(a.Difficulty):
		return errDifficulty
	case a.PhotoID != nil && *a.PhotoID != "" && uuid.Validate(*a.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a media id (UUID) from POST /v1/media")
	}

	for _, id := range a.GoalIDs {
		if uuid.Validate(id) != nil {
			return errs.Errf(errs.ErrValidation, "goal_ids must be goal ids (UUIDs), got %q", id)
		}
	}

	return nil
}
