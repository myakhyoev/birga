package tokens

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const userID = "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"

var cfg = config.JWTConfig{
	Secret: strings.Repeat("s", MinSecretLength), Issuer: "birga", AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour,
}

func TestNew_ShortSecret(t *testing.T) {
	short := cfg
	short.Secret = "short"

	if _, err := New(short); err == nil {
		t.Fatal("expected an error for a short secret")
	}
}

func TestIssueParse(t *testing.T) {
	iss, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	iss.now = func() time.Time { return now }

	access, _ := iss.Issue(userID, domain.UserRolePaidUser, domain.TokenTypeAccess)
	refresh, _ := iss.Issue(userID, domain.UserRolePaidUser, domain.TokenTypeRefresh)

	if c, err := iss.Parse(refresh, domain.TokenTypeRefresh); err != nil || c.UserID != userID || c.Role != domain.UserRolePaidUser {
		t.Fatalf("refresh: %+v %v", c, err)
	}

	if _, err := iss.Parse(access, domain.TokenTypeRefresh); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("access used as refresh: %v", err)
	}

	other := cfg
	other.Secret = strings.Repeat("x", MinSecretLength)
	foreign, _ := New(other)

	if _, err := foreign.Parse(refresh, domain.TokenTypeRefresh); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("other secret: %v", err)
	}

	if _, err := iss.Parse("garbage", domain.TokenTypeRefresh); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("garbage: %v", err)
	}

	// The access token expires after AccessTTL; the refresh token is still valid.
	now = now.Add(16 * time.Minute)

	if _, err := iss.Parse(access, domain.TokenTypeAccess); err == nil || err.Error() != "token expired" {
		t.Fatalf("expired access: %v", err)
	}

	if _, err := iss.Parse(refresh, domain.TokenTypeRefresh); err != nil {
		t.Fatalf("refresh after 16m: %v", err)
	}

	now = now.Add(cfg.RefreshTTL)

	if _, err := iss.Parse(refresh, domain.TokenTypeRefresh); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("expired refresh: %v", err)
	}
}
