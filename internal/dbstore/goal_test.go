package dbstore

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func TestGoalRepo_CRUD(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		created, err := s.Goal().Create(ctx, domain.Goal{NameUz: "Nutq test", NameRu: "Речь тест", NameEn: "Speech test"})
		if err != nil || created.ID == "" || created.CreatedAt.IsZero() {
			t.Fatalf("Create: %+v, %v", created, err)
		}

		updated, err := s.Goal().Update(ctx, created.ID, domain.GoalUpdate{NameEn: strPtr("Talking test")})
		if err != nil || updated.NameEn != "Talking test" || updated.NameUz != "Nutq test" {
			t.Fatalf("Update: %+v, %v", updated, err)
		}

		list, err := s.Goal().List(ctx)
		if err != nil || len(list) == 0 || list[len(list)-1].ID != created.ID {
			t.Fatalf("List: %+v, %v", list, err)
		}

		if n, err := s.Goal().CountActive(ctx, []string{created.ID, "00000000-0000-0000-0000-000000000000"}); err != nil || n != 1 {
			t.Fatalf("CountActive: %d, %v", n, err)
		}

		checkGoalDelete(ctx, t, s, created.ID)
	})
}

// checkGoalDelete deletes the goal, checks it is gone, and that its names are free again.
func checkGoalDelete(ctx context.Context, t *testing.T, s *DBStore, id string) {
	t.Helper()

	if err := s.Goal().Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := s.Goal().Get(ctx, id); !errors.Is(err, errs.ErrGoalNotFound) {
		t.Fatalf("Get after delete: %v", err)
	}

	if err := s.Goal().Delete(ctx, id); !errors.Is(err, errs.ErrGoalNotFound) {
		t.Fatalf("second Delete: %v", err)
	}

	if _, err := s.Goal().Create(ctx, domain.Goal{NameUz: "Nutq test", NameRu: "Речь тест", NameEn: "Talking test"}); err != nil {
		t.Fatalf("Create with freed names: %v", err)
	}
}

func TestGoalRepo_UniqueNames(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		if _, err := s.Goal().Create(ctx, domain.Goal{NameUz: "Harakat test", NameRu: "Моторика тест", NameEn: "Motor test"}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		_, err := s.Goal().Create(ctx, domain.Goal{NameUz: "Boshqa test", NameRu: "Другое тест", NameEn: "MOTOR TEST"})
		if !errors.Is(err, errs.ErrGoalNameTaken) || !errors.Is(err, errs.ErrConflict) {
			t.Fatalf("expected name taken, got %v", err)
		}
	})
}

func TestUserRepo_RelationshipAndGoals(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		goal, err := s.Goal().Create(ctx, domain.Goal{NameUz: "Ijtimoiy test", NameRu: "Социальное тест", NameEn: "Social test"})
		if err != nil {
			t.Fatalf("Create goal: %v", err)
		}

		u, err := s.User().Create(ctx, domain.User{
			Username: strPtr("goals_test"), PhoneNumber: strPtr("+998900000099"),
			Relationship: strPtr(domain.RelationshipNanny), GoalIDs: []string{goal.ID},
		})
		if err != nil {
			t.Fatalf("Create user: %v", err)
		}

		if *u.Relationship != domain.RelationshipNanny || len(u.GoalIDs) != 1 || u.GoalIDs[0] != goal.ID {
			t.Fatalf("unexpected user: %+v", u)
		}

		a, err := s.Activity().Create(ctx, activity(goal.ID, 2, 6, true))
		if err != nil {
			t.Fatalf("Create activity: %v", err)
		}

		// Deleting the goal removes it from the user and from the activity.
		if err := s.Goal().Delete(ctx, goal.ID); err != nil {
			t.Fatalf("Delete goal: %v", err)
		}

		got, err := s.User().Get(ctx, u.ID)
		if err != nil || got.GoalIDs == nil || len(got.GoalIDs) != 0 {
			t.Fatalf("Get user after goal delete: %+v, %v", got, err)
		}

		if got, err := s.Activity().Get(ctx, a.ID); err != nil || len(got.GoalIDs) != 0 {
			t.Fatalf("Get activity after goal delete: %+v, %v", got, err)
		}

		plain, err := s.User().Create(ctx, domain.User{Username: strPtr("nogoals_test"), PhoneNumber: strPtr("+998900000098")})
		if err != nil || plain.Relationship != nil || plain.GoalIDs == nil {
			t.Fatalf("Create plain user: %+v, %v", plain, err)
		}
	})
}
