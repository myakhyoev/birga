package postcreator

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type postRepo interface {
	Create(ctx context.Context, p domain.Post) (domain.Post, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

type mediaRepo interface {
	Get(ctx context.Context, id string) (domain.Media, error)
}

// UseCase validates and stores a new video post.
type UseCase struct {
	l     logger.Logger
	repo  postRepo
	goals goalRepo
	media mediaRepo
}

// New creates a new post creator use case.
func New(l logger.Logger, repo postRepo, goals goalRepo, media mediaRepo) *UseCase {
	return &UseCase{l: l, repo: repo, goals: goals, media: media}
}

// Execute validates business rules and persists the post. An empty status means in_review;
// goal_id must be an active goal and video_id an uploaded video.
func (uc *UseCase) Execute(ctx context.Context, p domain.Post) (domain.Post, error) {
	for _, s := range []*string{
		&p.TitleUz, &p.TitleRu, &p.TitleEn, &p.VideoID, &p.GoalID,
		&p.SuggestionUz, &p.SuggestionRu, &p.SuggestionEn, &p.QuestionsUz, &p.QuestionsRu, &p.QuestionsEn,
	} {
		*s = strings.TrimSpace(*s)
	}

	p.Status = strings.ToLower(strings.TrimSpace(p.Status))
	if p.Status == "" {
		p.Status = domain.ArticleStatusInReview
	}

	if err := validate(p); err != nil {
		return domain.Post{}, err
	}

	if !domain.NormalizeTopicID(p.TopicID) {
		return domain.Post{}, errs.Errf(errs.ErrValidation, "topic_id must be a topic id (UUID) from GET /v1/topics, or null for none")
	}

	if err := CheckRefs(ctx, uc.goals, uc.media, &p.GoalID, &p.VideoID); err != nil {
		return domain.Post{}, err
	}

	created, err := uc.repo.Create(ctx, p)
	if err != nil {
		return domain.Post{}, err
	}

	logger.WithContext(uc.l, ctx).Info("post created", zap.String("id", created.ID))

	return created, nil
}

// CheckRefs checks the ids that are set: goalID must be an active goal (errs.ErrUnknownGoals) and
// videoID an uploaded MP4 or WebM video (errs.ErrVideoNotFound).
func CheckRefs(ctx context.Context, goals goalRepo, media mediaRepo, goalID, videoID *string) error {
	if goalID != nil {
		n, err := goals.CountActive(ctx, []string{*goalID})
		if err != nil {
			return err
		}

		if n != 1 {
			return errs.ErrUnknownGoals
		}
	}

	if videoID != nil {
		m, err := media.Get(ctx, *videoID)
		if errors.Is(err, errs.ErrNotFound) || (err == nil && !domain.IsVideo(m.ContentType)) {
			return errs.ErrVideoNotFound
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// ValidateDuration checks a duration in minutes.
func ValidateDuration(d float64) error {
	if d <= 0 || d > domain.MaxPostDurationMinutes {
		return errs.Errf(errs.ErrValidation, "duration must be more than 0 and at most %d minutes", domain.MaxPostDurationMinutes)
	}

	return nil
}

func validate(p domain.Post) error {
	switch {
	case p.TitleUz == "" || p.TitleRu == "" || p.TitleEn == "":
		return errs.Errf(errs.ErrValidation, "title_uz, title_ru and title_en are required")
	case uuid.Validate(p.VideoID) != nil:
		return errs.Errf(errs.ErrValidation, "video_id must be a media id (UUID) from POST /v1/admin/media")
	case p.PhotoID != nil && *p.PhotoID != "" && uuid.Validate(*p.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a media id (UUID) of an uploaded image")
	case uuid.Validate(p.GoalID) != nil:
		return errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID) from GET /v1/goals")
	case p.MinAge < domain.MinChildAge || p.MaxAge > domain.MaxChildAge || p.MinAge > p.MaxAge:
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case !domain.IsKnownArticleStatus(p.Status):
		return errs.Errf(errs.ErrValidation, "status must be %s, %s or %s",
			domain.ArticleStatusPublished, domain.ArticleStatusInReview, domain.ArticleStatusUnpublished)
	}

	return ValidateDuration(p.Duration)
}
