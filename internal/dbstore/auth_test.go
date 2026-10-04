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
			UserID: u.ID, Username: "auth_test", PasswordHash: "bcrypt", Role: domain.UserRolePaidUser, AccessTokenHash: "a", RefreshTokenHash: "r",
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
		want := domain.UserAuth{
			UserID: u.ID, Username: "auth_renamed", PasswordHash: "bcrypt", Role: domain.UserRolePaidUser, AccessTokenHash: "a2", RefreshTokenHash: "r",
		}

		if err != nil || got != want {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, want)
		}

		if err := s.Auth().SetCredentials(ctx, domain.UserAuth{
			UserID: u.ID, PasswordHash: "bcrypt2", AccessTokenHash: "a4", RefreshTokenHash: "r4",
		}); err != nil {
			t.Fatalf("SetCredentials: %v", err)
		}

		if got, _ := s.Auth().Get(ctx, u.ID); got.PasswordHash != "bcrypt2" || got.AccessTokenHash != "a4" || got.RefreshTokenHash != "r4" {
			t.Fatalf("after SetCredentials: %+v", got)
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

		if err := s.Auth().SetCredentials(ctx, domain.UserAuth{UserID: u.ID}); !errors.Is(err, errs.ErrUserNotFound) {
			t.Fatalf("SetCredentials after delete: %v", err)
		}
	})
}

func TestAuthRepo_LoginLogout(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		u, err := s.User().Create(ctx, domain.User{Username: strPtr("login_test"), PhoneNumber: strPtr("+998900000012")})
		if err != nil {
			t.Fatalf("Create user: %v", err)
		}

		if err := s.Auth().Create(ctx, domain.UserAuth{UserID: u.ID, Username: "login_test", PasswordHash: "bcrypt", Role: domain.UserRoleUser}); err != nil {
			t.Fatalf("Create auth: %v", err)
		}

		if got, err := s.Auth().GetByUsername(ctx, "login_test"); err != nil || got.UserID != u.ID || got.PasswordHash != "bcrypt" {
			t.Fatalf("GetByUsername = %+v, %v", got, err)
		}

		if got, err := s.User().GetByPhone(ctx, "+998900000012"); err != nil || got.ID != u.ID {
			t.Fatalf("GetByPhone = %+v, %v", got, err)
		}

		if err := s.Auth().SetTokens(ctx, u.ID, "a5", "r5"); err != nil {
			t.Fatalf("SetTokens: %v", err)
		}

		if got, _ := s.Auth().Get(ctx, u.ID); got.AccessTokenHash != "a5" || got.RefreshTokenHash != "r5" || got.PasswordHash != "bcrypt" {
			t.Fatalf("after SetTokens: %+v", got)
		}

		// Clearing the tokens (logout) stores NULLs.
		if err := s.Auth().SetTokens(ctx, u.ID, "", ""); err != nil {
			t.Fatalf("SetTokens clear: %v", err)
		}

		if got, _ := s.Auth().Get(ctx, u.ID); got.AccessTokenHash != "" || got.RefreshTokenHash != "" {
			t.Fatalf("after clearing tokens: %+v", got)
		}

		checkGoneAfterDelete(ctx, t, s, u.ID)
	})
}

// checkGoneAfterDelete soft-deletes the user and checks that login lookups no longer find them.
func checkGoneAfterDelete(ctx context.Context, t *testing.T, s *DBStore, userID string) {
	t.Helper()

	if err := s.User().Delete(ctx, userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if err := s.Auth().SetTokens(ctx, userID, "", ""); !errors.Is(err, errs.ErrUserNotFound) {
		t.Fatalf("SetTokens after delete: %v", err)
	}

	if _, err := s.User().GetByPhone(ctx, "+998900000012"); !errors.Is(err, errs.ErrUserNotFound) {
		t.Fatalf("GetByPhone after delete: %v", err)
	}

	if _, err := s.Auth().GetByUsername(ctx, "login_test"); !errors.Is(err, errs.ErrUserNotFound) {
		t.Fatalf("GetByUsername after delete: %v", err)
	}
}
