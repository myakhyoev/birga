package dbstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func testVideo(t *testing.T, s *DBStore, ctx context.Context) string {
	t.Helper()

	id := uuid.NewString()

	m, err := s.Media().Create(ctx, domain.Media{ID: id, Key: "test/media/" + id + ".mp4", ContentType: "video/mp4", Size: 10})
	if err != nil {
		t.Fatalf("Create media: %v", err)
	}

	return m.ID
}

func post(videoID, goalID string, minAge, maxAge int, status string) domain.Post {
	return domain.Post{
		TitleUz: "Video", TitleRu: "Видео", TitleEn: "Video", VideoID: videoID, GoalID: goalID,
		MinAge: minAge, MaxAge: maxAge, Duration: 4.2, Status: status,
	}
}

func TestPostRepo_CRUDAndList(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `DELETE FROM posts`); err != nil {
			t.Fatalf("cleanup: %v", err)
		}

		goal, video := testGoal(t, s, ctx, "post crud"), testVideo(t, s, ctx)

		p := post(video, goal, 3, 5, domain.ArticleStatusInReview)
		p.QuestionsUz = "Bir?\nIkki?"

		created, err := s.Post().Create(ctx, p)
		if err != nil || created.Duration != 4.2 || created.QuestionsUz != "Bir?\nIkki?" || created.SuggestionEn != "" ||
			created.Source != domain.SourceAdmin {
			t.Fatalf("Create: %+v, %v", created, err)
		}

		upd, err := s.Post().Update(ctx, created.ID, domain.PostUpdate{
			Duration: func() *float64 { d := 3.67; return &d }(), Status: strPtr(domain.ArticleStatusPublished),
			Source: strPtr(domain.SourceHarvard),
		})
		if err != nil || upd.Duration != 3.67 || upd.Status != "published" || upd.TitleUz != "Video" ||
			upd.Source != domain.SourceHarvard {
			t.Fatalf("Update: %+v, %v", upd, err)
		}

		if _, err := s.Post().Create(ctx, post(video, goal, 2, 6, domain.ArticleStatusPublished)); err != nil {
			t.Fatalf("Create second: %v", err)
		}

		for name, tc := range map[string]struct {
			f     domain.PostFilter
			total int
		}{
			"all":       {domain.PostFilter{Limit: 10}, 2},
			"age 6":     {domain.PostFilter{Age: 6, Limit: 10}, 1},
			"published": {domain.PostFilter{Status: "published", Search: "vid", Limit: 10}, 2},
			"harvard":   {domain.PostFilter{Source: domain.SourceHarvard, Limit: 10}, 1},
			"ai":        {domain.PostFilter{Source: domain.SourceAI, Limit: 10}, 0},
		} {
			if _, total, err := s.Post().List(ctx, tc.f); err != nil || total != tc.total {
				t.Fatalf("%s: total=%d err=%v, want %d", name, total, err, tc.total)
			}
		}

		if err := s.Post().Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if _, err := s.Post().Get(ctx, created.ID); !errors.Is(err, errs.ErrPostNotFound) {
			t.Fatalf("deleted post found: %v", err)
		}
	})

	inRollbackTx(t, s, func(ctx context.Context) {
		missing := "00000000-0000-0000-0000-000000000000"
		if _, err := s.Post().Create(ctx, post(missing, testGoal(t, s, ctx, "post bad video"), 3, 5, "in_review")); !errors.Is(err, errs.ErrVideoNotFound) {
			t.Fatalf("unknown video: %v", err)
		}
	})
}

func TestPostRepo_Daily(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `DELETE FROM posts`); err != nil {
			t.Fatalf("cleanup: %v", err)
		}

		goal, other, video := testGoal(t, s, ctx, "post daily"), testGoal(t, s, ctx, "post daily other"), testVideo(t, s, ctx)
		mom, child := parentAndChild(t, s, ctx, 4)

		if _, err := s.Child().Update(ctx, child.ID, domain.ChildUpdate{GoalIDs: &[]string{goal}}); err != nil {
			t.Fatalf("set goals: %v", err)
		}

		repo := s.Post()

		offGoal, err := repo.Create(ctx, post(video, other, 2, 6, domain.ArticleStatusPublished))
		if err != nil {
			t.Fatal(err)
		}

		match, err := repo.Create(ctx, post(video, goal, 3, 5, domain.ArticleStatusPublished))
		if err != nil {
			t.Fatal(err)
		}

		id, ok, err := repo.PickDaily(ctx, mom.ID, nil)
		if err != nil || !ok || id != match.ID {
			t.Fatalf("pick: %s %v %v, want the goal match", id, ok, err)
		}

		day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
		if err := repo.SetDailyPost(ctx, mom.ID, day, id); err != nil {
			t.Fatal(err)
		}

		if got, ok, _ := repo.DailyPostID(ctx, mom.ID, day); !ok || got != match.ID {
			t.Fatalf("DailyPostID: %s %v", got, ok)
		}

		if next, _, _ := repo.PickDaily(ctx, mom.ID, []string{match.ID}); next != offGoal.ID {
			t.Fatalf("next: %s", next)
		}

		for d, want := range map[time.Time]bool{day: true, day.AddDate(0, 0, 1): true, day.AddDate(0, 0, -1): false} {
			if was, err := repo.WasDaily(ctx, mom.ID, match.ID, d); err != nil || was != want {
				t.Fatalf("WasDaily(%s) = %v %v, want %v", d, was, err, want)
			}
		}

		if hist, total, err := repo.DailyHistory(ctx, mom.ID, day, 10, 0); err != nil || total != 1 || hist[0].Post.ID != match.ID {
			t.Fatalf("DailyHistory: %d %+v %v", total, hist, err)
		}

		if err := repo.LockDaily(ctx, mom.ID); err != nil {
			t.Fatal(err)
		}
	})
}
