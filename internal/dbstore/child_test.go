package dbstore

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func intPtr(i int) *int { return &i }

func TestChildRepo_CRUD(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		created, err := s.Child().Create(ctx, domain.Child{Name: "Amir", Age: 3, Gender: domain.GenderMale})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if created.ID == "" || created.CreatedAt.IsZero() || created.PhotoID != nil {
			t.Fatalf("unexpected created child: %+v", created)
		}

		got, err := s.Child().Get(ctx, created.ID)
		if err != nil || got.Name != "Amir" || got.Age != 3 {
			t.Fatalf("Get: %+v, %v", got, err)
		}

		updated, err := s.Child().Update(ctx, created.ID, domain.ChildUpdate{Age: intPtr(4), Gender: strPtr(domain.GenderFemale)})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.Name != "Amir" || updated.Age != 4 || updated.Gender != domain.GenderFemale {
			t.Fatalf("unexpected updated child: %+v", updated)
		}
	})
}

func TestChildRepo_SoftDelete(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		created, err := s.Child().Create(ctx, domain.Child{Name: "Amir", Age: 3, Gender: domain.GenderMale})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := s.Child().Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if err := s.Child().Delete(ctx, created.ID); !errors.Is(err, errs.ErrChildNotFound) {
			t.Fatalf("second Delete: expected not found, got %v", err)
		}

		if _, err := s.Child().Get(ctx, created.ID); !errors.Is(err, errs.ErrChildNotFound) {
			t.Fatalf("Get after delete: expected not found, got %v", err)
		}

		if _, err := s.Child().Update(ctx, created.ID, domain.ChildUpdate{Name: strPtr("x")}); !errors.Is(err, errs.ErrChildNotFound) {
			t.Fatalf("Update after delete: expected not found, got %v", err)
		}
	})
}

func TestChildRepo_Invalid(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		check := func(name string, c domain.Child, want error) {
			if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "SAVEPOINT sp"); err != nil {
				t.Fatalf("savepoint: %v", err)
			}

			if _, err := s.Child().Create(ctx, c); !errors.Is(err, want) {
				t.Fatalf("%s: expected %v, got %v", name, want, err)
			}

			if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "ROLLBACK TO SAVEPOINT sp"); err != nil {
				t.Fatalf("rollback to savepoint: %v", err)
			}
		}

		check("age", domain.Child{Name: "a", Age: 30, Gender: domain.GenderMale}, errs.ErrValidation)
		check("gender", domain.Child{Name: "a", Age: 3, Gender: "x"}, errs.ErrValidation)
		check("photo", domain.Child{Name: "a", Age: 3, Gender: domain.GenderMale,
			PhotoID: strPtr("00000000-0000-0000-0000-000000000000")}, errs.ErrPhotoNotFound)
	})
}

func userIDs(users []domain.User) map[string]bool {
	ids := make(map[string]bool, len(users))
	for _, u := range users {
		ids[u.ID] = true
	}

	return ids
}

// family creates two parents, a child they share and a child only mom has.
func family(ctx context.Context, t *testing.T, s *DBStore) (mom, dad domain.User, shared, momsOnly domain.Child) {
	t.Helper()

	var err error

	if mom, err = s.User().Create(ctx, domain.User{PhoneNumber: strPtr("+998900000201")}); err != nil {
		t.Fatalf("Create mom: %v", err)
	}

	if dad, err = s.User().Create(ctx, domain.User{PhoneNumber: strPtr("+998900000202")}); err != nil {
		t.Fatalf("Create dad: %v", err)
	}

	if shared, err = s.Child().Create(ctx, domain.Child{Name: "First", Age: 5, Gender: domain.GenderFemale}); err != nil {
		t.Fatalf("Create shared: %v", err)
	}

	if momsOnly, err = s.Child().Create(ctx, domain.Child{Name: "Second", Age: 2, Gender: domain.GenderMale}); err != nil {
		t.Fatalf("Create momsOnly: %v", err)
	}

	// Linking the same pair twice is a no-op.
	for _, link := range [][2]string{{shared.ID, mom.ID}, {shared.ID, dad.ID}, {momsOnly.ID, mom.ID}, {momsOnly.ID, mom.ID}} {
		if err := s.Child().AddParent(ctx, link[0], link[1]); err != nil {
			t.Fatalf("AddParent %v: %v", link, err)
		}
	}

	return mom, dad, shared, momsOnly
}

func TestChildRepo_Parents(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		mom, dad, shared, _ := family(ctx, t, s)

		if items, total, err := s.Child().List(ctx, domain.ChildFilter{ParentID: mom.ID, Limit: 10}); err != nil || total != 2 || len(items) != 2 {
			t.Fatalf("List mom: %d %d %v", total, len(items), err)
		}

		if items, total, err := s.Child().List(ctx, domain.ChildFilter{ParentID: dad.ID, Limit: 10}); err != nil || total != 1 || items[0].ID != shared.ID {
			t.Fatalf("List dad: %d %+v %v", total, items, err)
		}

		// NOW() is fixed inside the test transaction, so both links share created_at; compare as a set.
		parents, err := s.Child().Parents(ctx, shared.ID)
		if got := userIDs(parents); err != nil || len(got) != 2 || !got[mom.ID] || !got[dad.ID] {
			t.Fatalf("Parents: %+v, %v", parents, err)
		}

		if err := s.Child().RemoveParent(ctx, shared.ID, dad.ID); err != nil {
			t.Fatalf("RemoveParent: %v", err)
		}

		if err := s.Child().RemoveParent(ctx, shared.ID, dad.ID); !errors.Is(err, errs.ErrChildNotFound) {
			t.Fatalf("second RemoveParent: %v", err)
		}
	})
}

func TestChildRepo_AddParentNotFound(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		mom, _, shared, _ := family(ctx, t, s)

		if err := s.Child().AddParent(ctx, "00000000-0000-0000-0000-000000000000", mom.ID); !errors.Is(err, errs.ErrChildNotFound) {
			t.Fatalf("AddParent unknown child: %v", err)
		}

		if err := s.Child().AddParent(ctx, shared.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("AddParent unknown user: %v", err)
		}
	})
}

// A deleted user drops out of Parents, and a deleted child out of List.
func TestChildRepo_DeletedHidden(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		mom, dad, shared, momsOnly := family(ctx, t, s)

		if err := s.User().Delete(ctx, mom.ID); err != nil {
			t.Fatalf("Delete mom: %v", err)
		}

		if parents, err := s.Child().Parents(ctx, shared.ID); err != nil || len(parents) != 1 || parents[0].ID != dad.ID {
			t.Fatalf("Parents after user delete: %+v, %v", parents, err)
		}

		if err := s.Child().AddParent(ctx, momsOnly.ID, dad.ID); err != nil {
			t.Fatalf("AddParent momsOnly: %v", err)
		}

		if err := s.Child().Delete(ctx, momsOnly.ID); err != nil {
			t.Fatalf("Delete momsOnly: %v", err)
		}

		if items, total, err := s.Child().List(ctx, domain.ChildFilter{ParentID: dad.ID, Limit: 10}); err != nil || total != 1 || items[0].ID != shared.ID {
			t.Fatalf("List dad after delete: %d %+v %v", total, items, err)
		}
	})
}
