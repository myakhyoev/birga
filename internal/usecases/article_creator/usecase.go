package articlecreator

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
	Create(ctx context.Context, a domain.Article) (domain.Article, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
}

// UseCase validates and stores a new article.
type UseCase struct {
	l     logger.Logger
	repo  articleRepo
	goals goalRepo
}

// New creates a new article creator use case.
func New(l logger.Logger, repo articleRepo, goals goalRepo) *UseCase {
	return &UseCase{
		l:     l,
		repo:  repo,
		goals: goals,
	}
}

// Execute validates business rules and persists the article. An empty status means in_review;
// goal_id must be an active goal and photo_id an uploaded media id.
func (uc *UseCase) Execute(ctx context.Context, a domain.Article) (domain.Article, error) {
	for _, s := range []*string{&a.TitleUz, &a.TitleRu, &a.TitleEn, &a.BodyUz, &a.BodyRu, &a.BodyEn, &a.TipUz, &a.TipRu, &a.TipEn} {
		*s = strings.TrimSpace(*s)
	}

	a.GoalID = strings.TrimSpace(a.GoalID)
	a.Status = strings.ToLower(strings.TrimSpace(a.Status))

	if a.Status == "" {
		a.Status = domain.ArticleStatusInReview
	}

	a.Source = domain.NormalizeSource(a.Source)
	if a.Source == "" {
		a.Source = domain.SourceAdmin
	}

	if !domain.ValidSource(a.Source) {
		return domain.Article{}, errs.ErrUnknownSource
	}

	if !domain.NormalizeTopicID(a.TopicID) {
		return domain.Article{}, errs.Errf(errs.ErrValidation, "topic_id must be a topic id (UUID) from GET /v1/topics, or null for none")
	}

	if err := validate(a); err != nil {
		return domain.Article{}, err
	}

	n, err := uc.goals.CountActive(ctx, []string{a.GoalID})
	if err != nil {
		return domain.Article{}, err
	}

	if n != 1 {
		return domain.Article{}, errs.ErrUnknownGoals
	}

	created, err := uc.repo.Create(ctx, a)
	if err != nil {
		return domain.Article{}, err
	}

	logger.WithContext(uc.l, ctx).Info("article created", zap.String("id", created.ID))

	return created, nil
}

func validate(a domain.Article) error {
	switch {
	case a.TitleUz == "" || a.TitleRu == "" || a.TitleEn == "":
		return errs.Errf(errs.ErrValidation, "title_uz, title_ru and title_en are required")
	case a.BodyUz == "" || a.BodyRu == "" || a.BodyEn == "":
		return errs.Errf(errs.ErrValidation, "article_body_uz, article_body_ru and article_body_en are required")
	case uuid.Validate(a.GoalID) != nil:
		return errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID) from GET /v1/goals")
	case a.PhotoID != nil && *a.PhotoID != "" && uuid.Validate(*a.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a media id (UUID) from POST /v1/media")
	case a.MinAge < domain.MinChildAge || a.MaxAge > domain.MaxChildAge || a.MinAge > a.MaxAge:
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case a.ReadMinutes <= 0 || a.ReadMinutes > domain.MaxArticleReadMinutes:
		return errs.Errf(errs.ErrValidation, "expected_read_time must be between 1 and %d minutes", domain.MaxArticleReadMinutes)
	case !domain.IsKnownArticleStatus(a.Status):
		return errs.Errf(errs.ErrValidation, "status must be %s, %s or %s",
			domain.ArticleStatusPublished, domain.ArticleStatusInReview, domain.ArticleStatusUnpublished)
	}

	return nil
}
