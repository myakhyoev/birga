package tokenchecker

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeTokens struct{}

func (fakeTokens) Parse(token string, want domain.TokenType) (domain.TokenClaims, error) {
	if token == "bad" || want != domain.TokenTypeAccess {
		return domain.TokenClaims{}, errs.Errf(errs.ErrUnauthorized, "invalid token")
	}

	return domain.TokenClaims{UserID: "u1", Type: want}, nil
}

type fakeAuth struct {
	a   domain.UserAuth
	err error
}

func (f fakeAuth) Get(context.Context, string) (domain.UserAuth, error) { return f.a, f.err }

func TestExecute(t *testing.T) {
	current := fakeAuth{a: domain.UserAuth{UserID: "u1", Role: domain.UserRolePaidUser, AccessTokenHash: domain.HashToken("good")}}

	p, err := New(nil, fakeTokens{}, current).Execute(context.Background(), " good ")
	if err != nil || p.UserID != "u1" || p.Role != domain.UserRolePaidUser {
		t.Fatalf("good token: %+v %v", p, err)
	}

	cases := map[string]struct {
		token string
		auth  fakeAuth
	}{
		"empty":        {"", current},
		"bad":          {"bad", current},
		"replaced":     {"older", current},
		"deleted user": {"good", fakeAuth{err: errs.ErrUserNotFound}},
	}

	for name, tc := range cases {
		if _, err := New(nil, fakeTokens{}, tc.auth).Execute(context.Background(), tc.token); !errors.Is(err, errs.ErrUnauthorized) {
			t.Errorf("%s: expected unauthorized, got %v", name, err)
		}
	}
}
