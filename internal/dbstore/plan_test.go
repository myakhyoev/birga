package dbstore

import (
	"context"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
)

func TestPlanRepo(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		hideOtherActivities(t, s, ctx)

		_, child := parentAndChild(t, s, ctx, 3)
		goal := testGoal(t, s, ctx, "plan test")
		other := testGoal(t, s, ctx, "plan other")

		plain, _ := s.Activity().Create(ctx, activity(other, 2, 6, true))
		served, _ := s.Activity().Create(ctx, activity(goal, 2, 6, true))

		both := activity(goal, 2, 6, true)
		both.GoalIDs = append(both.GoalIDs, other)
		both, _ = s.Activity().Create(ctx, both)

		// The day's goal first, then more of the child's goals.
		pick := domain.PlanPick{ChildID: child.ID, Age: 3, GoalID: &goal, ChildGoalIDs: []string{goal, other}, Seed: "1-1"}
		if got, err := s.Plan().Pick(ctx, pick); err != nil || got.ID != both.ID {
			t.Fatalf("Pick: %+v %v", got, err)
		}

		// Normal activities before easy or difficult ones, even ones serving more goals.
		easyBoth := activity(goal, 2, 6, true)
		easyBoth.GoalIDs, easyBoth.Difficulty = append(easyBoth.GoalIDs, other), domain.DifficultyEasy
		if _, err := s.Activity().Create(ctx, easyBoth); err != nil {
			t.Fatalf("Create easy: %v", err)
		}

		pick.ExcludeIDs = []string{both.ID}
		if got, err := s.Plan().Pick(ctx, pick); err != nil || got.ID != served.ID {
			t.Fatalf("Pick without both: %+v %v", got, err)
		}

		if plan, _, _, err := s.Plan().Current(ctx, child.ID); err != nil || plan.ID != "" {
			t.Fatalf("no plan yet: %+v %v", plan, err)
		}

		items := []domain.PlanItem{
			{Day: 1, GoalID: &goal, Activity: served},
			{Day: 2, GoalID: &other, Activity: plain},
			{Day: 3, Activity: both},
		}

		id, err := s.Plan().Create(ctx, child.ID, 1, oct3, items)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		plan, got, usable, err := s.Plan().Current(ctx, child.ID)
		if err != nil || plan.ID != id || plan.Number != 1 || !plan.StartedOn.Equal(oct3) || len(got) != 3 ||
			*got[1].GoalID != other || got[2].GoalID != nil || got[0].CompletedOn != nil || !usable[0] {
			t.Fatalf("Current: %+v %+v %v %v", plan, got, usable, err)
		}

		// Only the open day's activity counts, once per day.
		if day, err := s.Plan().MarkDone(ctx, child.ID, plain.ID, oct3); err != nil || day != 0 {
			t.Fatalf("not the open day: %d %v", day, err)
		}

		if day, err := s.Plan().MarkDone(ctx, child.ID, served.ID, oct3); err != nil || day != 1 {
			t.Fatalf("open day: %d %v", day, err)
		}

		if day, err := s.Plan().MarkDone(ctx, child.ID, served.ID, oct3); err != nil || day != 1 {
			t.Fatalf("same again: %d %v", day, err)
		}

		if day, err := s.Plan().MarkDone(ctx, child.ID, plain.ID, oct3); err != nil || day != 0 {
			t.Fatalf("second plan day on one day: %d %v", day, err)
		}

		// Two days later (one missed) the plan carries on with day 2.
		oct5 := oct3.AddDate(0, 0, 2)
		if day, err := s.Plan().MarkDone(ctx, child.ID, plain.ID, oct5); err != nil || day != 2 {
			t.Fatalf("after a missed day: %d %v", day, err)
		}

		days, total, first, err := s.Plan().DoneDays(ctx, child.ID)
		if err != nil || total != 2 || len(days) != 2 || !days[0].Equal(oct5) || !first.Equal(oct3) {
			t.Fatalf("DoneDays: %v %d %v %v", days, total, first, err)
		}

		if err := s.Plan().SetActivity(ctx, id, 3, served.ID); err != nil {
			t.Fatalf("SetActivity: %v", err)
		}

		// A newer plan becomes the current one; done days of the old one still count.
		id2, err := s.Plan().Create(ctx, child.ID, 2, oct5, items)
		if err != nil {
			t.Fatalf("Create 2: %v", err)
		}

		if plan, _, _, err := s.Plan().Current(ctx, child.ID); err != nil || plan.ID != id2 || plan.Number != 2 {
			t.Fatalf("Current 2: %+v %v", plan, err)
		}

		if _, total, _, err := s.Plan().DoneDays(ctx, child.ID); err != nil || total != 2 {
			t.Fatalf("DoneDays 2: %d %v", total, err)
		}

		if err := s.Plan().LockChild(ctx, child.ID); err != nil {
			t.Fatalf("LockChild: %v", err)
		}
	})
}
