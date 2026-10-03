package dbstore

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func strPtr(s string) *string { return &s }

func TestUserRepo_CRUD(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		created, err := s.User().Create(ctx, domain.User{
			Name: strPtr("Dilnoza"), Username: strPtr("dilnoza_test"), PhoneNumber: strPtr("+998900000001"),
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if created.ID == "" || created.CreatedAt.IsZero() || created.PhotoID != nil {
			t.Fatalf("unexpected created user: %+v", created)
		}

		got, err := s.User().Get(ctx, created.ID)
		if err != nil || *got.Username != "dilnoza_test" {
			t.Fatalf("Get: %+v, %v", got, err)
		}

		updated, err := s.User().Update(ctx, created.ID, domain.UserUpdate{Name: strPtr("Dili"), Username: strPtr("")})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if *updated.Name != "Dili" || updated.Username != nil || *updated.PhoneNumber != "+998900000001" {
			t.Fatalf("unexpected updated user: %+v", updated)
		}

		if _, err := s.User().Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}

func TestUserRepo_Conflicts(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		if _, err := s.User().Create(ctx, domain.User{Username: strPtr("dup_test"), PhoneNumber: strPtr("+998900000002")}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Each conflicting statement aborts the transaction, so check them in savepoints.
		check := func(name string, u domain.User, want error) {
			if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "SAVEPOINT sp"); err != nil {
				t.Fatalf("savepoint: %v", err)
			}

			if _, err := s.User().Create(ctx, u); !errors.Is(err, want) {
				t.Fatalf("%s: expected %v, got %v", name, want, err)
			}

			if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "ROLLBACK TO SAVEPOINT sp"); err != nil {
				t.Fatalf("rollback to savepoint: %v", err)
			}
		}

		check("username", domain.User{Username: strPtr("dup_test"), PhoneNumber: strPtr("+998900000003")}, errs.ErrUsernameTaken)
		check("phone", domain.User{PhoneNumber: strPtr("+998900000002")}, errs.ErrPhoneNumberTaken)
	})
}

func TestUserRepo_SoftDelete(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		u, err := s.User().Create(ctx, domain.User{Username: strPtr("gone_test"), PhoneNumber: strPtr("+998900000004")})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `INSERT INTO user_auth (id) VALUES ($1)`, u.ID); err != nil {
			t.Fatalf("insert user_auth: %v", err)
		}

		_, before, err := s.User().List(ctx, domain.UserFilter{Limit: 1})
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		if err := s.User().Delete(ctx, u.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if err := s.User().Delete(ctx, u.ID); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("second Delete: expected not found, got %v", err)
		}

		if _, err := s.User().Get(ctx, u.ID); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("Get after delete: expected not found, got %v", err)
		}

		if _, err := s.User().Update(ctx, u.ID, domain.UserUpdate{Name: strPtr("x")}); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("Update after delete: expected not found, got %v", err)
		}

		if _, after, _ := s.User().List(ctx, domain.UserFilter{Limit: 1}); after != before-1 {
			t.Fatalf("List total: before=%d after=%d", before, after)
		}

		var auths int
		if err := s.sqlClientByCtx(ctx).QueryRow(ctx, `SELECT COUNT(*) FROM user_auth WHERE id = $1`, u.ID).Scan(&auths); err != nil || auths != 0 {
			t.Fatalf("user_auth rows after delete: %d, %v", auths, err)
		}

		// The username and phone number are free again.
		if _, err := s.User().Create(ctx, domain.User{Username: strPtr("gone_test"), PhoneNumber: strPtr("+998900000004")}); err != nil {
			t.Fatalf("re-Create: %v", err)
		}
	})
}
