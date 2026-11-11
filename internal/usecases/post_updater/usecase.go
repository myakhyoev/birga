package postupdater

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	postcreator "gitlab.com/loyihalar/birga/backend/internal/usecases/post_creator"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type postRepo interface {
	Update(ctx context.Context, id string, upd domain.PostUpdate) (domain.Post, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

type mediaRepo interface {
	Get(ctx context.Context, id string) (domain.Media, error)
}

// UseCase applies a partial update to a post, including changing its status.
type UseCase struct {
	l     logger.Logger
	repo  postRepo
	goals goalRepo
	media mediaRepo
}

// New creates a new post updater use case.
func New(l logger.Logger, repo postRepo, goals goalRepo, media mediaRepo) *UseCase {
	return &UseCase{l: l, repo: repo, goals: goals, media: media}
}

// Execute validates the given fields and updates the post. Each field follows the create rules;
// an age range that only becomes invalid together with the stored value is rejected by the
// database check and also returned as a validation error.
func (uc *UseCase) Execute(ctx context.Context, id string, upd domain.PostUpdate) (domain.Post, error) {
	if upd.IsEmpty() {
		return domain.Post{}, errs.Errf(errs.ErrValidation, "nothing to update")
	}

	for _, s := range []**string{
		&upd.TitleUz, &upd.TitleRu, &upd.TitleEn, &upd.VideoID, &upd.GoalID,
		&upd.SuggestionUz, &upd.SuggestionRu, &upd.SuggestionEn, &upd.QuestionsUz, &upd.QuestionsRu, &upd.QuestionsEn,
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

	if err := validate(upd); err != nil {
		return domain.Post{}, err
	}

	if !domain.NormalizeTopicID(upd.TopicID) {
		return domain.Post{}, errs.Errf(errs.ErrValidation, "topic_id must be a topic id (UUID) from GET /v1/topics, or \"\" for none")
	}

	if err := postcreator.CheckRefs(ctx, uc.goals, uc.media, upd.GoalID, upd.VideoID); err != nil {
		return domain.Post{}, err
	}

	updated, err := uc.repo.Update(ctx, id, upd)
	if err != nil {
		return domain.Post{}, err
	}

	logger.WithContext(uc.l, ctx).Info("post updated", zap.String("id", id))

	return updated, nil
}

func validate(upd domain.PostUpdate) error {
	isBlank := func(s *string) bool { return s != nil && *s == "" }
	inRange := func(a *int) bool { return a == nil || (*a >= domain.MinChildAge && *a <= domain.MaxChildAge) }

	switch {
	case isBlank(upd.TitleUz) || isBlank(upd.TitleRu) || isBlank(upd.TitleEn):
		return errs.Errf(errs.ErrValidation, "title_uz, title_ru and title_en cannot be empty")
	case upd.VideoID != nil && uuid.Validate(*upd.VideoID) != nil:
		return errs.Errf(errs.ErrValidation, "video_id must be a media id (UUID) from POST /v1/admin/media")
	case upd.PhotoID != nil && *upd.PhotoID != "" && uuid.Validate(*upd.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a media id (UUID) of an uploaded image, or \"\" to remove the cover")
	case upd.GoalID != nil && uuid.Validate(*upd.GoalID) != nil:
		return errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID) from GET /v1/goals")
	case !inRange(upd.MinAge) || !inRange(upd.MaxAge) ||
		(upd.MinAge != nil && upd.MaxAge != nil && *upd.MinAge > *upd.MaxAge):
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case upd.Status != nil && !domain.IsKnownArticleStatus(*upd.Status):
		return errs.Errf(errs.ErrValidation, "status must be %s, %s or %s",
			domain.ArticleStatusPublished, domain.ArticleStatusInReview, domain.ArticleStatusUnpublished)
	case upd.Duration != nil:
		return postcreator.ValidateDuration(*upd.Duration)
	}

	return nil
}
