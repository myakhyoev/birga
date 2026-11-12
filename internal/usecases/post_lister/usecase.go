package postlister

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type postRepo interface {
	List(ctx context.Context, f domain.PostFilter) ([]domain.Post, int, error)
	DailyIDs(ctx context.Context, userID string, ids []string, day time.Time) (map[string]bool, error)
}

// UseCase lists posts with filtering and pagination.
type UseCase struct {
	l    logger.Logger
	repo postRepo
}

// New creates a new post lister use case.
func New(l logger.Logger, repo postRepo) *UseCase {
	return &UseCase{l: l, repo: repo}
}

// Execute returns a page of posts and the total number of matches.
func (uc *UseCase) Execute(ctx context.Context, f domain.PostFilter) ([]domain.Post, int, error) {
	f.Search = strings.TrimSpace(f.Search)
	f.Status = strings.ToLower(strings.TrimSpace(f.Status))
	f.Source = domain.NormalizeSource(f.Source)

	switch {
	case utf8.RuneCountInString(f.Search) > domain.MaxArticleSearchLength:
		return nil, 0, errs.Errf(errs.ErrValidation, "q must be at most %d characters", domain.MaxArticleSearchLength)
	case f.Status != "" && !domain.IsKnownArticleStatus(f.Status):
		return nil, 0, errs.Errf(errs.ErrValidation, "status must be %s, %s or %s",
			domain.ArticleStatusPublished, domain.ArticleStatusInReview, domain.ArticleStatusUnpublished)
	case f.Source != "" && !domain.ValidSource(f.Source):
		return nil, 0, errs.ErrUnknownSource
	case f.TopicID != "" && uuid.Validate(f.TopicID) != nil:
		return nil, 0, errs.Errf(errs.ErrValidation, "topic_id must be a topic id (UUID), got %q", f.TopicID)
	case f.GoalID != "" && uuid.Validate(f.GoalID) != nil:
		return nil, 0, errs.Errf(errs.ErrValidation, "goal_id must be a goal id (UUID), got %q", f.GoalID)
	case f.MaxMinutes < 0:
		return nil, 0, errs.Errf(errs.ErrValidation, "max_minutes must be positive")
	case f.Age != 0 && (f.Age < domain.MinChildAge || f.Age > domain.MaxChildAge):
		return nil, 0, errs.Errf(errs.ErrValidation, "age must be between %d and %d", domain.MinChildAge, domain.MaxChildAge)
	}

	items, total, err := uc.repo.List(ctx, f)
	if err != nil || f.LockFor == "" {
		return items, total, err
	}

	ids := make([]string, 0, len(items))
	for i := range items {
		ids = append(ids, items[i].ID)
	}

	opened, err := uc.repo.DailyIDs(ctx, f.LockFor, ids, domain.LocalDay(time.Now()))
	if err != nil {
		return nil, 0, err
	}

	for i := range items {
		items[i].Locked = !opened[items[i].ID]
	}

	return items, total, nil
}
