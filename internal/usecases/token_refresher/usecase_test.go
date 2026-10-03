package tokenrefresher

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const userID = "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"

type fakeTokens struct{}

func (fakeTokens) Issue(userID string, role domain.UserRole, typ domain.TokenType) (string, error) {
	return "new-" + string(typ) + "." + userID + "." + string(role), nil
}

func (fakeTokens) Parse(token string, want domain.TokenType) (domain.TokenClaims, error) {
	if token != "good-refresh" && token != "old-refresh" {
		return domain.TokenClaims{}, errs.Errf(errs.ErrUnauthorized, "invalid token")
	}

	return domain.TokenClaims{UserID: userID, Type: want}, nil
}

func (fakeTokens) AccessTTL() time.Duration { return 15 * time.Minute }

type fakeAuth struct {
	row       domain.UserAuth
	getErr    error
	savedHash string
}

func (f *fakeAuth) Get(context.Context, string) (domain.UserAuth, error) { return f.row, f.getErr }

func (f *fakeAuth) SetAccessToken(_ context.Context, _ string, hash string) error {
	f.savedHash = hash

	return nil
}

func TestExecute(t *testing.T) {
	auth := &fakeAuth{row: domain.UserAuth{UserID: userID, Role: domain.UserRolePaidUser, RefreshTokenHash: domain.HashToken("good-refresh")}}

	got, err := New(nil, fakeTokens{}, auth).Execute(context.Background(), " good-refresh ")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := "new-access." + userID + ".paid_user"
	if got.Token != want || got.ExpiresIn != 15*time.Minute || auth.savedHash != domain.HashToken(want) {
		t.Fatalf("got %+v, saved %q", got, auth.savedHash)
	}
}

func TestExecute_Rejected(t *testing.T) {
	cases := map[string]struct {
		token string
		auth  *fakeAuth
		want  error
	}{
		"empty":        {"", &fakeAuth{}, errs.ErrValidation},
		"bad jwt":      {"garbage", &fakeAuth{}, errs.ErrInvalidRefreshToken},
		"deleted user": {"good-refresh", &fakeAuth{getErr: errs.ErrUserNotFound}, errs.ErrInvalidRefreshToken},
		"replaced":     {"old-refresh", &fakeAuth{row: domain.UserAuth{RefreshTokenHash: domain.HashToken("good-refresh")}}, errs.ErrInvalidRefreshToken},
		"db down":      {"good-refresh", &fakeAuth{getErr: errs.ErrInternal}, errs.ErrInternal},
	}

	for name, tc := range cases {
		_, err := New(nil, fakeTokens{}, tc.auth).Execute(context.Background(), tc.token)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: got %v, want %v", name, err, tc.want)
		}

		if tc.auth.savedHash != "" {
			t.Fatalf("%s: access token saved", name)
		}
	}
}
