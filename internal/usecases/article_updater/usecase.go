package articleupdater

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type articleRepo interface {
	Update(ctx context.Context, id string, upd domain.ArticleUpdate) (domain.Article, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

// UseCase applies a partial update to an article, including changing its status.
type UseCase struct {
	l     logger.Logger
	repo  articleRepo
	goals goalRepo
}

// New creates a new article updater use case.
func New(l logger.Logger, repo articleRepo, goals goalRepo) *UseCase {
	return &UseCase{
		l:     l,
		repo:  repo,
		goals: goals,
	}
}

// Execute validates the given fields and updates the article. Each field follows the create rules;
// an age range that only becomes invalid together with the stored value is rejected by the
// database check and also returned as a validation error.
func (uc *UseCase) Execute(ctx context.Context, id string, upd domain.ArticleUpdate) (domain.Article, error) {
	if upd.IsEmpty() {
		return domain.Article{}, errs.Errf(errs.ErrValidation, "nothing to update")
	}

	for _, s := range []**string{
		&upd.TitleUz, &upd.TitleRu, &upd.TitleEn, &upd.BodyUz, &upd.BodyRu, &upd.BodyEn,
		&upd.TipUz, &upd.TipRu, &upd.TipEn, &upd.PhotoID, &upd.GoalID,
	} {
		if *s != nil {
			v := strings.TrimSpace(**s)
			*s = &v
		}
	}

	if upd.Status != nil {
		v := strings.ToLower(strings.TrimSpace(*upd.Status))
		upd.Status = &v
	}

	if upd.Source != nil {
		v := domain.NormalizeSource(*upd.Source)
		if !domain.ValidSource(v) {
			return domain.Article{}, errs.ErrUnknownSource
		}

		upd.Source = &v
	}

	if !domain.NormalizeTopicID(upd.TopicID) {
		return domain.Article{}, errs.Errf(errs.ErrValidation, "topic_id must be a topic id (UUID) from GET /v1/topics, or \"\" for none")
	}

	if err := validate(upd); err != nil {
		return domain.Article{}, err
	}

	if upd.GoalID != nil {
		n, err := uc.goals.CountActive(ctx, []string{*upd.GoalID})
		if err != nil {
			return domain.Article{}, err
		}

		if n != 1 {
			return domain.Article{}, errs.ErrUnknownGoals
		}
	}

	updated, err := uc.repo.Update(ctx, id, upd)
	if err != nil {
		return domain.Article{}, err
	}

	logger.WithContext(uc.l, ctx).Info("article updated", zap.String("id", id))

	return updated, nil
}

func validate(upd domain.ArticleUpdate) error {
	switch {
	case isBlank(upd.TitleUz) || isBlank(upd.TitleRu) || isBlank(upd.TitleEn):
		return errs.Errf(errs.ErrValidation, "title_uz, title_ru and title_en cannot be empty")
	case isBlank(upd.BodyUz) || isBlank(upd.BodyRu) || isBlank(upd.BodyEn):
		return errs.Errf(errs.ErrValidation, "article_body_uz, article_body_ru and article_body_en cannot be empty")
	case upd.GoalID != nil && uuid.Validate(*upd.GoalID) != nil:
		return errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID) from GET /v1/goals")
	case upd.PhotoID != nil && *upd.PhotoID != "" && uuid.Validate(*upd.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a media id (UUID) from POST /v1/media, or \"\" to remove the photo")
	case !validAges(upd.MinAge, upd.MaxAge):
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case upd.ReadMinutes != nil && (*upd.ReadMinutes <= 0 || *upd.ReadMinutes > domain.MaxArticleReadMinutes):
		return errs.Errf(errs.ErrValidation, "expected_read_time must be between 1 and %d minutes", domain.MaxArticleReadMinutes)
	case upd.Status != nil && !domain.IsKnownArticleStatus(*upd.Status):
		return errs.Errf(errs.ErrValidation, "status must be %s, %s or %s",
			domain.ArticleStatusPublished, domain.ArticleStatusInReview, domain.ArticleStatusUnpublished)
	}

	return nil
}

func isBlank(s *string) bool { return s != nil && *s == "" }

// validAges checks the ages that are set; comparing with stored values is left to the database check.
func validAges(minAge, maxAge *int) bool {
	inRange := func(a *int) bool { return a == nil || (*a >= domain.MinChildAge && *a <= domain.MaxChildAge) }

	return inRange(minAge) && inRange(maxAge) && (minAge == nil || maxAge == nil || *minAge <= *maxAge)
}
