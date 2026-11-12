package dbstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func article(goalID string, minAge, maxAge int, status string) domain.Article {
	return domain.Article{
		TitleUz: "Sarlavha", TitleRu: "Заголовок", TitleEn: "Title", BodyUz: "b", BodyRu: "б", BodyEn: "b",
		ReadMinutes: 3, MinAge: minAge, MaxAge: maxAge, GoalID: goalID, Status: status,
	}
}

// hideOtherArticles removes rows that may already exist in the dev database, inside the test transaction.
func hideOtherArticles(t *testing.T, s *DBStore, ctx context.Context) {
	t.Helper()

	if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `DELETE FROM articles`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestArticleRepo_CRUD(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		goal := testGoal(t, s, ctx, "article crud")

		a := article(goal, 2, 5, domain.ArticleStatusInReview)
		a.TipEn = "try it"

		created, err := s.Article().Create(ctx, a)
		if err != nil || created.ID == "" || created.TipEn != "try it" || created.PhotoID != nil || created.Status != "in_review" ||
			created.Source != domain.SourceAdmin {
			t.Fatalf("Create: %+v, %v", created, err)
		}

		upd, err := s.Article().Update(ctx, created.ID, domain.ArticleUpdate{
			Status: strPtr(domain.ArticleStatusPublished), MinAge: intPtr(3), TipEn: strPtr(""), Source: strPtr(domain.SourceAI),
		})
		if err != nil || upd.Status != "published" || upd.MinAge != 3 || upd.TipEn != "" || upd.TitleUz != "Sarlavha" ||
			upd.Source != domain.SourceAI {
			t.Fatalf("Update: %+v, %v", upd, err)
		}

		if _, total, err := s.Article().List(ctx, domain.ArticleFilter{Source: domain.SourceAI, Limit: 10}); err != nil || total < 1 {
			t.Fatalf("List source=ai: total=%d, %v", total, err)
		}

		if err := s.Article().Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if _, err := s.Article().Get(ctx, created.ID, ""); !errors.Is(err, errs.ErrArticleNotFound) {
			t.Fatalf("deleted article still found: %v", err)
		}

		if err := s.Article().Delete(ctx, created.ID); !errors.Is(err, errs.ErrArticleNotFound) {
			t.Fatalf("second delete: %v", err)
		}
	})

	// Each failing write aborts its transaction, so each gets its own.
	for name, try := range map[string]func(ctx context.Context, repo *articleRepo, a domain.Article, id string) error{
		"unknown photo": func(ctx context.Context, repo *articleRepo, a domain.Article, _ string) error {
			missing := "00000000-0000-0000-0000-000000000000"
			a.PhotoID = &missing
			_, err := repo.Create(ctx, a)

			return err
		},
		"min_age above stored max_age": func(ctx context.Context, repo *articleRepo, _ domain.Article, id string) error {
			_, err := repo.Update(ctx, id, domain.ArticleUpdate{MinAge: intPtr(6)})

			return err
		},
		"unknown goal": func(ctx context.Context, repo *articleRepo, _ domain.Article, id string) error {
			_, err := repo.Update(ctx, id, domain.ArticleUpdate{GoalID: strPtr("00000000-0000-0000-0000-000000000000")})

			return err
		},
	} {
		inRollbackTx(t, s, func(ctx context.Context) {
			a := article(testGoal(t, s, ctx, "article invalid"), 2, 5, domain.ArticleStatusInReview)

			created, err := s.Article().Create(ctx, a)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			if err := try(ctx, s.Article(), a, created.ID); !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("%s: want a validation error, got %v", name, err)
			}
		})
	}
}

func TestArticleRepo_ListAndRead(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		hideOtherArticles(t, s, ctx)

		goal, other := testGoal(t, s, ctx, "article list"), testGoal(t, s, ctx, "article list other")
		mom, _ := parentAndChild(t, s, ctx, 3)

		var ids []string

		for _, a := range []domain.Article{
			article(goal, 2, 3, domain.ArticleStatusPublished),
			article(goal, 4, 6, domain.ArticleStatusPublished),
			article(other, 2, 6, domain.ArticleStatusInReview),
		} {
			created, err := s.Article().Create(ctx, a)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			ids = append(ids, created.ID)
		}

		cases := []struct {
			name  string
			f     domain.ArticleFilter
			total int
		}{
			{"all", domain.ArticleFilter{Limit: 10}, 3},
			{"published", domain.ArticleFilter{Status: "published", Limit: 10}, 2},
			{"goal", domain.ArticleFilter{GoalID: other, Limit: 10}, 1},
			{"age 5", domain.ArticleFilter{Age: 5, Limit: 10}, 2},
			{"search", domain.ArticleFilter{Search: "title", Limit: 10}, 3},
			{"reader", domain.ArticleFilter{Status: "published", ReaderID: mom.ID, Limit: 1}, 2},
		}

		for _, tc := range cases {
			if _, total, err := s.Article().List(ctx, tc.f); err != nil || total != tc.total {
				t.Fatalf("%s: total=%d err=%v, want %d", tc.name, total, err, tc.total)
			}
		}

		if err := s.Article().MarkRead(ctx, mom.ID, ids[0]); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}

		if err := s.Article().MarkRead(ctx, mom.ID, ids[0]); err != nil {
			t.Fatalf("MarkRead again: %v", err)
		}

		if err := s.Article().MarkRead(ctx, mom.ID, ids[2]); !errors.Is(err, errs.ErrArticleNotFound) {
			t.Fatalf("MarkRead of an in_review article: %v", err)
		}

		got, err := s.Article().Get(ctx, ids[0], mom.ID)
		if err != nil || !got.IsRead {
			t.Fatalf("Get as reader: %+v, %v", got, err)
		}

		if got, _ := s.Article().Get(ctx, ids[1], mom.ID); got.IsRead {
			t.Fatalf("unread article shown read")
		}

		items, _, err := s.Article().List(ctx, domain.ArticleFilter{GoalID: goal, ReaderID: mom.ID, Limit: 10})
		if err != nil || len(items) != 2 {
			t.Fatalf("List as reader: %v", err)
		}

		for _, it := range items {
			if it.IsRead != (it.ID == ids[0]) {
				t.Fatalf("is_read wrong for %s: %v", it.ID, it.IsRead)
			}
		}
	})
}

func TestArticleRepo_Daily(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		hideOtherArticles(t, s, ctx)

		sleep, talk, other := testGoal(t, s, ctx, "daily sleep"), testGoal(t, s, ctx, "daily talk"), testGoal(t, s, ctx, "daily other")
		mom, child := parentAndChild(t, s, ctx, 3)

		if _, err := s.Child().Update(ctx, child.ID, domain.ChildUpdate{GoalIDs: &[]string{sleep, talk}}); err != nil {
			t.Fatalf("set goals: %v", err)
		}

		repo := s.Article()

		if _, ok, err := repo.PickDaily(ctx, mom.ID, nil); err != nil || ok {
			t.Fatalf("pick with no articles: %v %v", ok, err)
		}

		// NOW() is fixed inside a transaction, so each article gets its own created_at to order by.
		created := 0
		create := func(goal string, minAge, maxAge int, status string) string {
			a, err := repo.Create(ctx, article(goal, minAge, maxAge, status))
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			created++
			if _, err := s.sqlClientByCtx(ctx).Exec(ctx,
				`UPDATE articles SET created_at = NOW() + make_interval(secs => $2) WHERE id = $1`, a.ID, created); err != nil {
				t.Fatalf("set created_at: %v", err)
			}

			return a.ID
		}

		offGoal := create(other, 2, 6, domain.ArticleStatusPublished)
		tooOld := create(sleep, 5, 6, domain.ArticleStatusPublished)
		sleep1 := create(sleep, 2, 4, domain.ArticleStatusPublished)
		sleep2 := create(sleep, 2, 4, domain.ArticleStatusPublished)
		talk1 := create(talk, 3, 3, domain.ArticleStatusPublished)
		create(talk, 2, 6, domain.ArticleStatusInReview)

		day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)

		// Picks a day after day, storing each pick like the use case does.
		var got []string

		for i := range 6 {
			id, ok, err := repo.PickDaily(ctx, mom.ID, nil)
			if err != nil || !ok {
				t.Fatalf("pick %d: %v %v", i, ok, err)
			}

			if err := repo.SetDailyArticle(ctx, mom.ID, day.AddDate(0, 0, i), id); err != nil {
				t.Fatalf("SetDailyArticle: %v", err)
			}

			got = append(got, id)
		}

		// Only articles that fit both a goal and the age while any is left, rotating goals (sleep1,
		// talk1, sleep2), then the one shown longest ago repeats.
		want := []string{sleep1, talk1, sleep2, sleep1, talk1, sleep2}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("day %d: got %s, want %s (order %v)", i, got[i], want[i], got)
			}
		}

		// Without those, the goal fit with the wrong age comes next, then any other article.
		if id, _, _ := repo.PickDaily(ctx, mom.ID, []string{sleep1, sleep2, talk1}); id != tooOld {
			t.Fatalf("goal fit with the wrong age: got %s, want %s", id, tooOld)
		}

		if id, _, _ := repo.PickDaily(ctx, mom.ID, []string{sleep1, sleep2, talk1, tooOld}); id != offGoal {
			t.Fatalf("other goal: got %s, want %s", id, offGoal)
		}

		if id, ok, err := repo.DailyArticleID(ctx, mom.ID, day.AddDate(0, 0, 1)); err != nil || !ok || id != talk1 {
			t.Fatalf("DailyArticleID: %s %v %v", id, ok, err)
		}

		if _, err := repo.Update(ctx, talk1, domain.ArticleUpdate{Status: strPtr(domain.ArticleStatusUnpublished)}); err != nil {
			t.Fatalf("unpublish: %v", err)
		}

		if _, ok, _ := repo.DailyArticleID(ctx, mom.ID, day.AddDate(0, 0, 1)); ok {
			t.Fatal("an unpublished daily article must read as missing")
		}

		if err := repo.Delete(ctx, sleep2); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Days 0..4 hold sleep1, talk1, sleep2, sleep1, talk1; sleep2 is deleted, talk1 only unpublished.
		hist, total, err := repo.DailyHistory(ctx, mom.ID, day.AddDate(0, 0, 4), 2, 0)
		if err != nil || total != 4 || len(hist) != 2 || hist[0].Article.ID != talk1 || !hist[0].Day.Equal(day.AddDate(0, 0, 4)) {
			t.Fatalf("DailyHistory: total=%d %+v %v", total, hist, err)
		}

		if err := repo.LockDaily(ctx, mom.ID); err != nil {
			t.Fatalf("LockDaily: %v", err)
		}
	})
}
