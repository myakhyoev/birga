package activityupdater

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type activityRepo interface {
	Update(ctx context.Context, id string, upd domain.ActivityUpdate) (domain.Activity, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

// UseCase applies a partial update to an activity, including publishing and unpublishing it.
type UseCase struct {
	l     logger.Logger
	repo  activityRepo
	goals goalRepo
}

// New creates a new activity updater use case.
func New(l logger.Logger, repo activityRepo, goals goalRepo) *UseCase {
	return &UseCase{
		l:     l,
		repo:  repo,
		goals: goals,
	}
}

// Execute validates the given fields and updates the activity. Each field follows the create rules;
// an age range that only becomes invalid together with the stored value (for example min_age above
// the stored max_age) is rejected by the database check and also returned as a validation error.
func (uc *UseCase) Execute(ctx context.Context, id string, upd domain.ActivityUpdate) (domain.Activity, error) {
	if upd.IsEmpty() {
		return domain.Activity{}, errs.Errf(errs.ErrValidation, "nothing to update")
	}

	for _, s := range []**string{&upd.TitleUz, &upd.TitleRu, &upd.TitleEn, &upd.DescriptionUz, &upd.DescriptionRu, &upd.DescriptionEn,
		&upd.MainIdeaUz, &upd.MainIdeaRu, &upd.MainIdeaEn,
		&upd.EasierUz, &upd.EasierRu, &upd.EasierEn, &upd.HarderUz, &upd.HarderRu, &upd.HarderEn} {
		if *s != nil {
			v := strings.TrimSpace(**s)
			*s = &v
		}
	}

	if upd.GoalIDs != nil {
		upd.GoalIDs = domain.NormalizeIDs(upd.GoalIDs)
	}

	if upd.Difficulty != nil {
		v := strings.ToLower(strings.TrimSpace(*upd.Difficulty))
		upd.Difficulty = &v
	}

	if upd.Source != nil {
		v := domain.NormalizeSource(*upd.Source)
		if !domain.ValidSource(v) {
			return domain.Activity{}, errs.ErrUnknownSource
		}

		upd.Source = &v
	}

	if err := validate(upd); err != nil {
		return domain.Activity{}, err
	}

	if upd.GoalIDs != nil {
		n, err := uc.goals.CountActive(ctx, upd.GoalIDs)
		if err != nil {
			return domain.Activity{}, err
		}

		if n != len(upd.GoalIDs) {
			return domain.Activity{}, errs.ErrUnknownGoals
		}
	}

	updated, err := uc.repo.Update(ctx, id, upd)
	if err != nil {
		return domain.Activity{}, err
	}

	logger.WithContext(uc.l, ctx).Info("activity updated", zap.String("id", id))

	return updated, nil
}

func validate(upd domain.ActivityUpdate) error {
	switch {
	case isBlank(upd.TitleUz) || isBlank(upd.TitleRu) || isBlank(upd.TitleEn):
		return errs.Errf(errs.ErrValidation, "title_uz, title_ru and title_en cannot be empty")
	case isBlank(upd.DescriptionUz) || isBlank(upd.DescriptionRu) || isBlank(upd.DescriptionEn):
		return errs.Errf(errs.ErrValidation, "description_uz, description_ru and description_en cannot be empty")
	case upd.GoalIDs != nil && (len(upd.GoalIDs) == 0 || len(upd.GoalIDs) > domain.MaxActivityGoals):
		return errs.Errf(errs.ErrValidation, "goal_ids must hold 1 to %d goal ids", domain.MaxActivityGoals)
	case !validAges(upd.MinAge, upd.MaxAge):
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case upd.DurationMinutes != nil &&
		(*upd.DurationMinutes <= 0 || *upd.DurationMinutes > domain.MaxActivityDurationMinutes):
		return errs.Errf(errs.ErrValidation, "duration_minutes must be between 1 and %d", domain.MaxActivityDurationMinutes)
	case upd.Difficulty != nil && !domain.ValidDifficulty(*upd.Difficulty):
		return errs.Errf(errs.ErrValidation, "difficulty must be %s, %s or %s",
			domain.DifficultyEasy, domain.DifficultyNormal, domain.DifficultyDifficult)
	case upd.PhotoID != nil && *upd.PhotoID != "" && uuid.Validate(*upd.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a media id (UUID) from POST /v1/media, or \"\" to remove the photo")
	}

	for _, id := range upd.GoalIDs {
		if uuid.Validate(id) != nil {
			return errs.Errf(errs.ErrValidation, "goal_ids must be goal ids (UUIDs), got %q", id)
		}
	}

	return nil
}

func isBlank(s *string) bool { return s != nil && *s == "" }

// validAges checks the ages that are set; comparing with stored values is left to the database check.
func validAges(minAge, maxAge *int) bool {
	inRange := func(a *int) bool { return a == nil || (*a >= domain.MinChildAge && *a <= domain.MaxChildAge) }

	return inRange(minAge) && inRange(maxAge) && (minAge == nil || maxAge == nil || *minAge <= *maxAge)
}
