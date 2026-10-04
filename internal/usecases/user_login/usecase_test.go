package userlogin

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeAuth struct {
	hash         string
	gotUsername  string
	setAccess    string
	setRefresh   string
	setForUserID string
}

func (f *fakeAuth) GetByUsername(_ context.Context, username string) (domain.UserAuth, error) {
	f.gotUsername = username
	if username != "dilnoza_k" {
		return domain.UserAuth{}, errs.ErrUserNotFound
	}

	return domain.UserAuth{UserID: "u1", Username: username, PasswordHash: f.hash, Role: domain.UserRolePaidUser}, nil
}

func (f *fakeAuth) SetTokens(_ context.Context, userID, access, refresh string) error {
	f.setForUserID, f.setAccess, f.setRefresh = userID, access, refresh

	return nil
}

type fakeTokens struct{ roles []domain.UserRole }

func (f *fakeTokens) Issue(_ string, role domain.UserRole, typ domain.TokenType) (string, error) {
	f.roles = append(f.roles, role)

	return string(typ) + "-token", nil
}

func (f *fakeTokens) AccessTTL() time.Duration { return time.Hour }

func newAuth(t *testing.T) *fakeAuth {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	return &fakeAuth{hash: string(hash)}
}

func TestExecute(t *testing.T) {
	a, tk := newAuth(t), &fakeTokens{}

	pair, err := New(nil, a, tk).Execute(context.Background(), "  Dilnoza_K ", "s3cret-pass")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if pair.AccessToken != "access-token" || pair.RefreshToken != "refresh-token" || pair.AccessExpiresIn != time.Hour {
		t.Fatalf("pair %+v", pair)
	}

	if a.gotUsername != "dilnoza_k" || a.setForUserID != "u1" ||
		a.setAccess != domain.HashToken("access-token") || a.setRefresh != domain.HashToken("refresh-token") {
		t.Fatalf("auth %+v", a)
	}

	if tk.roles[0] != domain.UserRolePaidUser {
		t.Fatalf("roles %v", tk.roles)
	}
}

func TestExecute_InvalidCredentials(t *testing.T) {
	for _, tc := range []struct{ username, password string }{
		{"dilnoza_k", "wrong-pass"},
		{"nobody", "s3cret-pass"},
	} {
		a := newAuth(t)
		if _, err := New(nil, a, &fakeTokens{}).Execute(context.Background(), tc.username, tc.password); !errors.Is(err, errs.ErrInvalidCredentials) || a.setForUserID != "" {
			t.Fatalf("%+v: %v", tc, err)
		}
	}

	// A user without a stored password cannot sign in.
	a := &fakeAuth{}
	if _, err := New(nil, a, &fakeTokens{}).Execute(context.Background(), "dilnoza_k", "s3cret-pass"); !errors.Is(err, errs.ErrInvalidCredentials) {
		t.Fatalf("no password: %v", err)
	}
}

func TestExecute_Missing(t *testing.T) {
	if _, err := New(nil, newAuth(t), &fakeTokens{}).Execute(context.Background(), " ", ""); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
