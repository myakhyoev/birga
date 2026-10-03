package rest

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// fakeMe records what the /v1/me handlers pass to the use cases.
type fakeMe struct {
	gotUserID   string
	gotUpdate   domain.UserUpdate
	gotPassword string
	gotFilter   domain.ChildFilter
	gotChildID  string
	err         error
}

func (f *fakeMe) update(userID string, upd domain.UserUpdate) (domain.User, error) {
	f.gotUserID, f.gotUpdate = userID, upd

	return domain.User{ID: userID, Name: upd.Name}, f.err
}

func (f *fakeMe) resetPassword(userID, password string) (domain.TokenPair, error) {
	f.gotUserID, f.gotPassword = userID, password

	return domain.TokenPair{AccessToken: "a3", RefreshToken: "r3", AccessExpiresIn: time.Hour}, f.err
}

func (f *fakeMe) listChildren(userID string, fl domain.ChildFilter) ([]domain.Child, int, error) {
	f.gotUserID, f.gotFilter = userID, fl

	return []domain.Child{{ID: testChildID, Name: "Amir"}}, 1, f.err
}

func (f *fakeMe) getChild(userID, childID string) (domain.Child, error) {
	f.gotUserID, f.gotChildID = userID, childID

	return domain.Child{ID: childID}, f.err
}

type (
	profileUpdaterFunc   func(string, domain.UserUpdate) (domain.User, error)
	passwordResetterFunc func(string, string) (domain.TokenPair, error)
	childListerFunc      func(string, domain.ChildFilter) ([]domain.Child, int, error)
	childGetterFunc      func(string, string) (domain.Child, error)
)

func (fn profileUpdaterFunc) Execute(_ context.Context, userID string, upd domain.UserUpdate) (domain.User, error) {
	return fn(userID, upd)
}

func (fn passwordResetterFunc) Execute(_ context.Context, userID, password string) (domain.TokenPair, error) {
	return fn(userID, password)
}

func (fn childListerFunc) Execute(_ context.Context, userID string, f domain.ChildFilter) ([]domain.Child, int, error) {
	return fn(userID, f)
}

func (fn childGetterFunc) Execute(_ context.Context, userID, childID string) (domain.Child, error) {
	return fn(userID, childID)
}

func TestMeRequiresToken(t *testing.T) {
	s, _ := newTestServer("")

	for _, r := range [][2]string{
		{http.MethodGet, "/v1/me"}, {http.MethodPatch, "/v1/me"}, {http.MethodDelete, "/v1/me"},
		{http.MethodPut, "/v1/me/password"}, {http.MethodGet, "/v1/me/children"}, {http.MethodGet, "/v1/children/" + testChildID},
	} {
		if code, _ := do(t, s, r[0], r[1], "{}", nil); code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", r[0], r[1], code)
		}
	}
}

func TestGetProfile(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodGet, "/v1/me", "", map[string]string{authorizationHeader: "Bearer admin"})
	if code != http.StatusOK || d.users.gotID != testUserID {
		t.Fatalf("got %d %+v, id %q", code, r, d.users.gotID)
	}

	data, _ := r.Data.(map[string]any)
	if data["id"] != testUserID || data["role"] != string(domain.UserRoleAdmin) {
		t.Fatalf("data %+v", data)
	}
}

func TestUpdateProfile(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPatch, "/v1/me", `{"name":"Dilnoza","phone_number":"+998901234567"}`, bearer)
	if code != http.StatusOK || d.me.gotUserID != testUserID || *d.me.gotUpdate.Name != "Dilnoza" ||
		*d.me.gotUpdate.PhoneNumber != "+998901234567" || d.me.gotUpdate.Username != nil {
		t.Fatalf("got %d %+v, update %+v", code, r, d.me.gotUpdate)
	}

	if data, _ := r.Data.(map[string]any); data["role"] != string(domain.UserRoleUser) {
		t.Fatalf("data %+v", data)
	}

	if code, _ := do(t, s, http.MethodPatch, "/v1/me", "{broken", bearer); code != http.StatusBadRequest {
		t.Fatalf("broken json: %d", code)
	}

	d.me.err = errs.ErrNewPhoneNotVerified

	if code, _ := do(t, s, http.MethodPatch, "/v1/me", `{"phone_number":"+998901234568"}`, bearer); code != http.StatusForbidden {
		t.Fatalf("not verified: %d", code)
	}
}

func TestResetPassword(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPut, "/v1/me/password", `{"password":"n3w-s3cret"}`, bearer)
	if code != http.StatusOK || d.me.gotUserID != testUserID || d.me.gotPassword != "n3w-s3cret" {
		t.Fatalf("got %d %+v", code, r)
	}

	if data, _ := r.Data.(map[string]any); data["access_token"] != "a3" || data["refresh_token"] != "r3" || data["expires_in"] != float64(3600) {
		t.Fatalf("data %+v", data)
	}

	d.me.err = errs.ErrResetNotVerified

	if code, _ := do(t, s, http.MethodPut, "/v1/me/password", `{"password":"n3w-s3cret"}`, bearer); code != http.StatusForbidden {
		t.Fatalf("not verified: %d", code)
	}
}

func TestDeleteAccount(t *testing.T) {
	s, d := newTestServer("")

	if code, r := do(t, s, http.MethodDelete, "/v1/me", "", bearer); code != http.StatusOK || d.users.gotID != testUserID {
		t.Fatalf("got %d %+v, id %q", code, r, d.users.gotID)
	}
}

func TestListMyChildren(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodGet, "/v1/me/children?limit=5&offset=10", "", bearer)
	if code != http.StatusOK || d.me.gotUserID != testUserID || d.me.gotFilter != (domain.ChildFilter{Limit: 5, Offset: 10}) {
		t.Fatalf("got %d %+v, filter %+v", code, r, d.me.gotFilter)
	}

	if data, _ := r.Data.(map[string]any); data["total"] != float64(1) {
		t.Fatalf("data %+v", data)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/me/children?limit=x", "", bearer); code != http.StatusBadRequest {
		t.Fatalf("bad limit: %d", code)
	}
}

func TestGetChild(t *testing.T) {
	s, d := newTestServer("")

	if code, r := do(t, s, http.MethodGet, "/v1/children/"+testChildID, "", bearer); code != http.StatusOK ||
		d.me.gotUserID != testUserID || d.me.gotChildID != testChildID {
		t.Fatalf("got %d %+v", code, r)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/children/nope", "", bearer); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}

	d.me.err = errs.ErrChildNotFound

	if code, _ := do(t, s, http.MethodGet, "/v1/children/"+testChildID, "", bearer); code != http.StatusNotFound {
		t.Fatalf("not found: %d", code)
	}
}
