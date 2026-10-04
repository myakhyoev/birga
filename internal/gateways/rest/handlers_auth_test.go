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
	gotLogin   [2]string
	gotLogout  string
	gotForgot  [2]string
	err        error
}

func (f *fakeAuth) login(username, password string) (domain.TokenPair, error) {
	f.gotLogin = [2]string{username, password}

	return domain.TokenPair{AccessToken: "a", RefreshToken: "r", AccessExpiresIn: 15 * time.Minute}, f.err
}

func (f *fakeAuth) logout(userID string) error {
	f.gotLogout = userID

	return f.err
}

func (f *fakeAuth) forgot(phone, password string) (domain.TokenPair, error) {
	f.gotForgot = [2]string{phone, password}

	return domain.TokenPair{AccessToken: "a", RefreshToken: "r", AccessExpiresIn: 15 * time.Minute}, f.err
}

type loginFunc func(string, string) (domain.TokenPair, error)

func (fn loginFunc) Execute(_ context.Context, username, password string) (domain.TokenPair, error) {
	return fn(username, password)
}

type logoutFunc func(string) error

func (fn logoutFunc) Execute(_ context.Context, userID string) error { return fn(userID) }

type forgotFunc func(string, string) (domain.TokenPair, error)

func (fn forgotFunc) ExecuteByPhone(_ context.Context, phone, password string) (domain.TokenPair, error) {
	return fn(phone, password)
}

func (f *fakeAuth) Execute(_ context.Context, req domain.SignUpRequest) (domain.TokenPair, error) {
	f.got = req

	return domain.TokenPair{AccessToken: "a", RefreshToken: "r", AccessExpiresIn: 15 * time.Minute}, f.err
}

func (f *fakeAuth) refresh(token string) (domain.AccessToken, error) {
	f.gotRefresh = token

	return domain.AccessToken{Token: "a2", ExpiresIn: 15 * time.Minute}, f.err
}

// check accepts the access token "good" as user testUserID with role user, and "admin" as an admin.
func (f *fakeAuth) check(token string) (domain.Principal, error) {
	switch token {
	case "good":
		return domain.Principal{UserID: testUserID, Role: domain.UserRoleUser}, nil
	case "admin":
		return domain.Principal{UserID: testUserID, Role: domain.UserRoleAdmin}, nil
	default:
		return domain.Principal{}, errs.Errf(errs.ErrUnauthorized, "invalid token")
	}
}

type checkerFunc func(string) (domain.Principal, error)

func (fn checkerFunc) Execute(_ context.Context, token string) (domain.Principal, error) {
	return fn(token)
}

type refresherFunc func(string) (domain.AccessToken, error)

func (fn refresherFunc) Execute(_ context.Context, token string) (domain.AccessToken, error) {
	return fn(token)
}

func TestSignUp(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPost, "/v1/auth/signup",
		`{"name": "Dilnoza", "username": "dilnoza_k", "password": "s3cret-pass", "phone_number": "+998901234567", "user_role": "paid_user"}`, nil)
	if code != http.StatusOK {
		t.Fatalf("got %d %+v", code, r)
	}

	want := domain.SignUpRequest{Name: "Dilnoza", Username: "dilnoza_k", Password: "s3cret-pass", PhoneNumber: "+998901234567", Role: domain.UserRolePaidUser}
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

func TestLogin(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPost, "/v1/auth/login", `{"username": "dilnoza_k", "password": "s3cret-pass"}`, nil)
	if code != http.StatusOK || d.auth.gotLogin != [2]string{"dilnoza_k", "s3cret-pass"} {
		t.Fatalf("got %d %+v, login %v", code, r, d.auth.gotLogin)
	}

	if data, _ := r.Data.(map[string]any); data["access_token"] != "a" || data["refresh_token"] != "r" || data["expires_in"] != float64(900) {
		t.Fatalf("unexpected data: %+v", r.Data)
	}

	d.auth.err = errs.ErrInvalidCredentials

	if code, r := do(t, s, http.MethodPost, "/v1/auth/login", `{"username": "x", "password": "y"}`, nil); code != http.StatusUnauthorized || r.ErrorCode != _errCodeUnauthorized {
		t.Fatalf("wrong password: %d %+v", code, r)
	}

	if code, _ := do(t, s, http.MethodPost, "/v1/auth/login", `{`, nil); code != http.StatusBadRequest {
		t.Fatalf("bad JSON: %d", code)
	}
}

func TestLogout(t *testing.T) {
	s, d := newTestServer("")

	if code, _ := do(t, s, http.MethodPost, "/v1/auth/logout", "", nil); code != http.StatusUnauthorized || d.auth.gotLogout != "" {
		t.Fatalf("no token: %d", code)
	}

	code, r := do(t, s, http.MethodPost, "/v1/auth/logout", "", map[string]string{"Authorization": "Bearer good"})
	if code != http.StatusOK || d.auth.gotLogout != testUserID {
		t.Fatalf("got %d %+v, logout %q", code, r, d.auth.gotLogout)
	}
}

func TestForgotPassword(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPost, "/v1/auth/forgot-password", `{"phone_number": "+998901234567", "password": "n3w-s3cret-pass"}`, nil)
	if code != http.StatusOK || d.auth.gotForgot != [2]string{"+998901234567", "n3w-s3cret-pass"} {
		t.Fatalf("got %d %+v, forgot %v", code, r, d.auth.gotForgot)
	}

	if data, _ := r.Data.(map[string]any); data["access_token"] != "a" {
		t.Fatalf("unexpected data: %+v", r.Data)
	}

	for err, want := range map[error]int{
		errs.ErrResetNotVerified:         http.StatusForbidden,
		errs.ErrPhoneNumberNotRegistered: http.StatusNotFound,
	} {
		d.auth.err = err
		if code, _ := do(t, s, http.MethodPost, "/v1/auth/forgot-password", `{}`, nil); code != want {
			t.Fatalf("%v: got %d, want %d", err, code, want)
		}
	}
}
