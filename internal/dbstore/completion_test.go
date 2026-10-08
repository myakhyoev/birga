package dbstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func boolPtr(b bool) *bool { return &b }

// hideOtherActivities unpublishes every committed activity inside the rolled-back test transaction,
// so recommendation tests only see the rows they create.
func hideOtherActivities(t *testing.T, s *DBStore, ctx context.Context) {
	t.Helper()

	if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `UPDATE activities SET is_published = FALSE`); err != nil {
		t.Fatalf("hide activities: %v", err)
	}
}

func TestActivityRepo_Update(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		a, err := s.Activity().Create(ctx, activity(testGoal(t, s, ctx, "motor test"), 2, 4, false))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		updated, err := s.Activity().Update(ctx, a.ID, domain.ActivityUpdate{IsPublished: boolPtr(true), MaxAge: intPtr(5)})
		if err != nil || !updated.IsPublished || updated.MaxAge != 5 || updated.MinAge != 2 {
			t.Fatalf("Update: %+v, %v", updated, err)
		}

		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "SAVEPOINT sp"); err != nil {
			t.Fatalf("savepoint: %v", err)
		}

		if _, err := s.Activity().Update(ctx, a.ID, domain.ActivityUpdate{MinAge: intPtr(6)}); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("min_age above max_age: expected validation error, got %v", err)
		}

		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "ROLLBACK TO SAVEPOINT sp"); err != nil {
			t.Fatalf("rollback to savepoint: %v", err)
		}
	})
}

func TestActivityRepo_Delete(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		a, err := s.Activity().Create(ctx, activity(testGoal(t, s, ctx, "motor test"), 2, 4, true))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := s.Activity().Delete(ctx, a.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if err := s.Activity().Delete(ctx, a.ID); !errors.Is(err, errs.ErrActivityNotFound) {
			t.Fatalf("second Delete: %v", err)
		}

		if _, err := s.Activity().Get(ctx, a.ID); !errors.Is(err, errs.ErrActivityNotFound) {
			t.Fatalf("Get after delete: %v", err)
		}

		if _, err := s.Activity().Update(ctx, a.ID, domain.ActivityUpdate{IsPublished: boolPtr(false)}); !errors.Is(err, errs.ErrActivityNotFound) {
			t.Fatalf("Update after delete: %v", err)
		}

		items, _, err := s.Activity().List(ctx, domain.ActivityFilter{Limit: 1000})
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		for _, it := range items {
			if it.ID == a.ID {
				t.Fatalf("deleted activity listed")
			}
		}
	})
}

var oct3 = time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)

func TestCompletionRepo_SameDayIsIdempotent(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		mom, child := parentAndChild(t, s, ctx, 4)
		a := publishedActivity(t, s, ctx)

		first, created, err := s.Completion().Create(ctx, domain.Completion{
			ChildID: child.ID, ActivityID: a.ID, UserID: &mom.ID, CompletedOn: oct3, Note: strPtr("zo'r"),
		})
		if err != nil || !created || first.CompletedOn.Format(time.DateOnly) != "2026-10-03" || *first.Note != "zo'r" {
			t.Fatalf("Create: %+v created=%v %v", first, created, err)
		}

		again, created, err := s.Completion().Create(ctx, domain.Completion{ChildID: child.ID, ActivityID: a.ID, CompletedOn: oct3})
		if err != nil || created || again.ID != first.ID || again.Note == nil || *again.Note != "zo'r" {
			t.Fatalf("same day again: %+v created=%v %v", again, created, err)
		}
	})
}

func TestCompletionRepo_DaysAndList(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		_, child := parentAndChild(t, s, ctx, 4)
		a := publishedActivity(t, s, ctx)

		for _, d := range []time.Time{oct3, oct3.AddDate(0, 0, -1), oct3.AddDate(0, 0, -3)} {
			if _, _, err := s.Completion().Create(ctx, domain.Completion{ChildID: child.ID, ActivityID: a.ID, CompletedOn: d}); err != nil {
				t.Fatalf("Create %v: %v", d, err)
			}
		}

		days, total, err := s.Completion().Days(ctx, child.ID)
		if err != nil || total != 3 || len(days) != 3 || !days[0].Equal(oct3) || !days[2].Equal(oct3.AddDate(0, 0, -3)) {
			t.Fatalf("Days: %v total=%d %v", days, total, err)
		}

		page, total, err := s.Completion().List(ctx, domain.CompletionFilter{ChildID: child.ID, Limit: 2})
		if err != nil || total != 3 || len(page) != 2 || !page[0].CompletedOn.Equal(oct3) {
			t.Fatalf("List: %+v total=%d %v", page, total, err)
		}
	})
}

func publishedActivity(t *testing.T, s *DBStore, ctx context.Context) domain.Activity {
	t.Helper()

	a, err := s.Activity().Create(ctx, activity(testGoal(t, s, ctx, "social test"), 2, 6, true))
	if err != nil {
		t.Fatalf("Create activity: %v", err)
	}

	return a
}

func TestActivityRepo_Recommend(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		hideOtherActivities(t, s, ctx)

		_, child := parentAndChild(t, s, ctx, 3)
		q := domain.RecommendationQuery{ChildID: child.ID, Age: 3, Day: oct3}

		if _, err := s.Activity().Recommend(ctx, q); !errors.Is(err, errs.ErrNoRecommendation) {
			t.Fatalf("empty catalogue: %v", err)
		}

		language, motor := testGoal(t, s, ctx, "language test"), testGoal(t, s, ctx, "motor test")

		short := activity(language, 2, 4, true)
		short.DurationMinutes = 5

		a, _ := s.Activity().Create(ctx, short)
		b, _ := s.Activity().Create(ctx, activity(language, 3, 5, true))
		_, _ = s.Activity().Create(ctx, activity(language, 5, 6, true))  // too old for 3
		_, _ = s.Activity().Create(ctx, activity(language, 2, 4, false)) // unpublished

		first, err := s.Activity().Recommend(ctx, q)
		if err != nil || (first.ID != a.ID && first.ID != b.ID) {
			t.Fatalf("Recommend: %+v %v", first, err)
		}

		if again, _ := s.Activity().Recommend(ctx, q); again.ID != first.ID {
			t.Fatalf("pick changed within a day: %s then %s", first.ID, again.ID)
		}

		// Once the pick is done, the other, never-done activity comes first.
		if _, _, err := s.Completion().Create(ctx, domain.Completion{ChildID: child.ID, ActivityID: first.ID, CompletedOn: oct3}); err != nil {
			t.Fatalf("Complete: %v", err)
		}

		next, err := s.Activity().Recommend(ctx, q)
		if err != nil || next.ID == first.ID {
			t.Fatalf("after completion: %+v %v", next, err)
		}

		q.MaxMinutes = 5
		if got, err := s.Activity().Recommend(ctx, q); err != nil || got.ID != a.ID {
			t.Fatalf("time limit: %+v %v", got, err)
		}

		q.MaxMinutes, q.GoalID = 0, motor
		if _, err := s.Activity().Recommend(ctx, q); !errors.Is(err, errs.ErrNoRecommendation) {
			t.Fatalf("goal filter: %v", err)
		}

		// An activity serving a preferred goal wins over never-done ones without it, even once done.
		c, _ := s.Activity().Create(ctx, activity(motor, 2, 4, true))
		if _, _, err := s.Completion().Create(ctx, domain.Completion{ChildID: child.ID, ActivityID: c.ID, CompletedOn: oct3}); err != nil {
			t.Fatalf("Complete: %v", err)
		}

		q.GoalID, q.PreferGoalIDs = "", []string{motor}
		if got, err := s.Activity().Recommend(ctx, q); err != nil || got.ID != c.ID {
			t.Fatalf("preferred goal: %+v %v", got, err)
		}

		q.PreferGoalIDs = nil
		if got, err := s.Activity().Recommend(ctx, q); err != nil || got.ID == c.ID {
			t.Fatalf("no preference: %+v %v", got, err)
		}
	})
}

func TestChildRepo_GetForParent(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		mom, child := parentAndChild(t, s, ctx, 4)

		if got, err := s.Child().GetForParent(ctx, child.ID, mom.ID); err != nil || got.ID != child.ID {
			t.Fatalf("own child: %+v %v", got, err)
		}

		stranger, err := s.User().Create(ctx, domain.User{PhoneNumber: strPtr("+998900000302")})
		if err != nil {
			t.Fatalf("Create stranger: %v", err)
		}

		if _, err := s.Child().GetForParent(ctx, child.ID, stranger.ID); !errors.Is(err, errs.ErrChildNotFound) {
			t.Fatalf("someone else's child: %v", err)
		}
	})
}

func parentAndChild(t *testing.T, s *DBStore, ctx context.Context, age int) (domain.User, domain.Child) {
	t.Helper()

	mom, err := s.User().Create(ctx, domain.User{PhoneNumber: strPtr("+998900000301")})
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}

	child, err := s.Child().Create(ctx, domain.Child{Name: "Laylo", Age: age, Gender: domain.GenderFemale})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}

	if err := s.Child().AddParent(ctx, child.ID, mom.ID); err != nil {
		t.Fatalf("AddParent: %v", err)
	}

	return mom, child
}
