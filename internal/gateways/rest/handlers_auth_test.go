package rest

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeAuth struct {
	got        domain.SignUpRequest
	gotRefresh string
	err        error
}

func (f *fakeAuth) Execute(_ context.Context, req domain.SignUpRequest) (domain.TokenPair, error) {
	f.got = req

	return domain.TokenPair{AccessToken: "a", RefreshToken: "r", AccessExpiresIn: 15 * time.Minute}, f.err
}

func (f *fakeAuth) refresh(token string) (domain.AccessToken, error) {
	f.gotRefresh = token

	return domain.AccessToken{Token: "a2", ExpiresIn: 15 * time.Minute}, f.err
}

type refresherFunc func(string) (domain.AccessToken, error)

func (fn refresherFunc) Execute(_ context.Context, token string) (domain.AccessToken, error) {
	return fn(token)
}

func TestSignUp(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPost, "/v1/auth/signup",
		`{"name": "Dilnoza", "username": "dilnoza_k", "password": "s3cret-pass", "phone_number": "+998901234567"}`, nil)
	if code != http.StatusOK {
		t.Fatalf("got %d %+v", code, r)
	}

	want := domain.SignUpRequest{Name: "Dilnoza", Username: "dilnoza_k", Password: "s3cret-pass", PhoneNumber: "+998901234567"}
	if d.auth.got != want {
		t.Fatalf("request = %+v, want %+v", d.auth.got, want)
	}

	data, _ := r.Data.(map[string]any)
	if data["access_token"] != "a" || data["refresh_token"] != "r" || data["expires_in"] != float64(900) {
		t.Fatalf("unexpected data: %+v", r.Data)
	}

	d.auth.err = errs.ErrPhoneNotVerified

	if code, r := do(t, s, http.MethodPost, "/v1/auth/signup", `{}`, nil); code != http.StatusForbidden || r.ErrorCode != _errCodeForbidden {
		t.Fatalf("not verified: %d %+v", code, r)
	}

	if code, _ := do(t, s, http.MethodPost, "/v1/auth/signup", `{`, nil); code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", code)
	}
}

func TestRefreshToken(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPost, "/v1/auth/refresh", `{"refresh_token": "r"}`, nil)
	if code != http.StatusOK || d.auth.gotRefresh != "r" {
		t.Fatalf("got %d %+v, token %q", code, r, d.auth.gotRefresh)
	}

	data, _ := r.Data.(map[string]any)
	if data["access_token"] != "a2" || data["expires_in"] != float64(900) {
		t.Fatalf("unexpected data: %+v", r.Data)
	}

	d.auth.err = errs.ErrInvalidRefreshToken

	if code, r := do(t, s, http.MethodPost, "/v1/auth/refresh", `{"refresh_token": "x"}`, nil); code != http.StatusUnauthorized || r.ErrorCode != _errCodeUnauthorized {
		t.Fatalf("invalid: %d %+v", code, r)
	}
}
