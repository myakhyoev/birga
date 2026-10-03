package dbstore

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func TestAuthRepo(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		u, err := s.User().Create(ctx, domain.User{Username: strPtr("auth_test"), PhoneNumber: strPtr("+998900000011")})
		if err != nil {
			t.Fatalf("Create user: %v", err)
		}

		if err := s.Auth().Create(ctx, domain.UserAuth{
			UserID: u.ID, Username: "auth_test", PasswordHash: "bcrypt", AccessTokenHash: "a", RefreshTokenHash: "r",
		}); err != nil {
			t.Fatalf("Create auth: %v", err)
		}

		if err := s.Auth().SetAccessToken(ctx, u.ID, "a2"); err != nil {
			t.Fatalf("SetAccessToken: %v", err)
		}

		// Renaming the user renames the sign-in username (users_sync_auth_username_trg).
		if _, err := s.User().Update(ctx, u.ID, domain.UserUpdate{Username: strPtr("auth_renamed")}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := s.Auth().Get(ctx, u.ID)
		want := domain.UserAuth{UserID: u.ID, Username: "auth_renamed", PasswordHash: "bcrypt", AccessTokenHash: "a2", RefreshTokenHash: "r"}

		if err != nil || got != want {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, want)
		}

		// A soft delete removes the auth row.
		if err := s.User().Delete(ctx, u.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if _, err := s.Auth().Get(ctx, u.ID); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("Get after delete: %v", err)
		}

		if err := s.Auth().SetAccessToken(ctx, u.ID, "a3"); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("SetAccessToken after delete: %v", err)
		}
	})
}
