package dbstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func hideOtherMilestones(t *testing.T, s *DBStore, ctx context.Context) {
	t.Helper()

	if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `UPDATE milestones SET deleted_at = NOW()`); err != nil {
		t.Fatalf("hide milestones: %v", err)
	}
}

func milestone(dom string, ageMonths, sortOrder int, status string) domain.Milestone {
	return domain.Milestone{Domain: dom, AgeMonths: ageMonths, SortOrder: sortOrder,
		TextUz: "uz", TextRu: "ru", TextEn: "en", ActivityIDs: []string{}, Status: status}
}

func TestMilestoneSeed(t *testing.T) {
	s := newTestStore(t)

	stages, err := s.Milestone().Stages(context.Background())
	if err != nil || len(stages) < 5 || stages[0] != 24 {
		t.Fatalf("seeded stages: %v %v", stages, err)
	}
}

func TestMilestoneRepo_CRUD(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		hideOtherMilestones(t, s, ctx)

		repo := s.Milestone()
		act := publishedActivity(t, s, ctx)

		in := milestone(domain.MilestoneMovement, 36, 2, domain.ArticleStatusInReview)
		in.ActivityIDs, in.HowToUz = []string{act.ID}, "1-qadam\n2-qadam"

		m, err := repo.Create(ctx, in)
		if err != nil || m.ID == "" || m.HowToUz != "1-qadam\n2-qadam" || len(m.ActivityIDs) != 1 {
			t.Fatalf("Create: %+v %v", m, err)
		}

		upd, err := repo.Update(ctx, m.ID, domain.MilestoneUpdate{Status: strPtr("published"), ActivityIDs: []string{}, AgeMonths: intPtr(48)})
		if err != nil || upd.Status != "published" || len(upd.ActivityIDs) != 0 || upd.AgeMonths != 48 || upd.HowToUz == "" {
			t.Fatalf("Update: %+v %v", upd, err)
		}

		if _, err := repo.Create(ctx, milestone(domain.MilestoneSocial, 48, 1, domain.ArticleStatusPublished)); err != nil {
			t.Fatal(err)
		}

		items, total, err := repo.List(ctx, domain.MilestoneFilter{AgeMonths: 48, Limit: 10})
		if err != nil || total != 2 || items[0].Domain != domain.MilestoneSocial || items[1].Domain != domain.MilestoneMovement {
			t.Fatalf("List order: %d %+v %v", total, items, err)
		}

		if _, total, err := repo.List(ctx, domain.MilestoneFilter{Domain: domain.MilestoneMovement, Search: "EN", Status: "published", Limit: 10}); err != nil || total != 1 {
			t.Fatalf("List filters: %d %v", total, err)
		}

		if stages, err := repo.Stages(ctx); err != nil || len(stages) != 1 || stages[0] != 48 {
			t.Fatalf("Stages: %v %v", stages, err)
		}

		if err := repo.Delete(ctx, m.ID); err != nil {
			t.Fatal(err)
		}

		if _, err := repo.Get(ctx, m.ID); !errors.Is(err, errs.ErrMilestoneNotFound) {
			t.Fatalf("deleted: %v", err)
		}

		// Last: a failed CHECK aborts the transaction.
		if _, err := repo.Update(ctx, items[0].ID, domain.MilestoneUpdate{Domain: strPtr("nope")}); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("bad domain: %v", err)
		}
	})
}

func TestMilestoneRepo_Child(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		hideOtherMilestones(t, s, ctx)

		var (
			repo       = s.Milestone()
			mom, child = parentAndChild(t, s, ctx, 4)
		)

		create := func(dom string, status string) string {
			m, err := repo.Create(ctx, milestone(dom, 48, 1, status))
			if err != nil {
				t.Fatal(err)
			}

			return m.ID
		}

		talk := create(domain.MilestoneCommunication, domain.ArticleStatusPublished)
		create(domain.MilestoneMovement, domain.ArticleStatusPublished)
		draft := create(domain.MilestoneThinking, domain.ArticleStatusInReview)

		for range 2 {
			if err := repo.Mark(ctx, child.ID, talk, mom.ID); err != nil {
				t.Fatal(err)
			}
		}

		items, err := repo.ForChild(ctx, child.ID, 48)
		if err != nil || len(items) != 2 || items[0].ID != talk || !items[0].Achieved || items[0].AchievedAt == nil || items[1].Achieved {
			t.Fatalf("ForChild: %+v %v", items, err)
		}

		if m, err := repo.GetForChild(ctx, draft, child.ID); err != nil || m.Status != domain.ArticleStatusInReview || m.Achieved {
			t.Fatalf("GetForChild: %+v %v", m, err)
		}

		now := time.Now()
		if n, err := repo.CountAchieved(ctx, child.ID, now.Add(-time.Hour), now.Add(time.Hour)); err != nil || n != 1 {
			t.Fatalf("CountAchieved: %d %v", n, err)
		}

		if err := repo.Unmark(ctx, child.ID, talk); err != nil {
			t.Fatal(err)
		}

		if m, err := repo.GetForChild(ctx, talk, child.ID); err != nil || m.Achieved {
			t.Fatalf("after Unmark: %+v %v", m, err)
		}
	})
}

func TestActivityRepo_ListByIDsAndCount(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		goal := testGoal(t, s, ctx, "by ids")

		pub, err := s.Activity().Create(ctx, activity(goal, 2, 6, true))
		if err != nil {
			t.Fatal(err)
		}

		draft, err := s.Activity().Create(ctx, activity(goal, 2, 6, false))
		if err != nil {
			t.Fatal(err)
		}

		ids := []string{draft.ID, pub.ID, "00000000-0000-0000-0000-000000000000"}

		if got, err := s.Activity().ListByIDs(ctx, ids, false); err != nil || len(got) != 2 || got[0].ID != draft.ID {
			t.Fatalf("all: %+v %v", got, err)
		}

		if got, err := s.Activity().ListByIDs(ctx, ids, true); err != nil || len(got) != 1 || got[0].ID != pub.ID {
			t.Fatalf("published: %+v %v", got, err)
		}

		_, child := parentAndChild(t, s, ctx, 3)
		day := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

		for _, d := range []time.Time{day, day.AddDate(0, 0, 1)} {
			if _, _, err := s.Completion().Create(ctx, domain.Completion{ChildID: child.ID, ActivityID: pub.ID, CompletedOn: d}); err != nil {
				t.Fatal(err)
			}
		}

		if n, err := s.Completion().CountBetween(ctx, child.ID, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), day); err != nil || n != 1 {
			t.Fatalf("CountBetween: %d %v", n, err)
		}
	})
}
