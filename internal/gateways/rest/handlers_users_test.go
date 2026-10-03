package rest

import (
	"context"
	"net/http"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// fakeUsers records what the user handlers pass to the use cases.
type fakeUsers struct {
	gotUser   domain.User
	gotFilter domain.UserFilter
	gotID     string
	gotUpdate domain.UserUpdate
	err       error
}

func (f *fakeUsers) create(_ context.Context, u domain.User) (domain.User, error) {
	f.gotUser = u
	u.ID = testID

	return u, f.err
}

func (f *fakeUsers) list(_ context.Context, fl domain.UserFilter) ([]domain.User, int, error) {
	f.gotFilter = fl

	return []domain.User{{ID: testID}}, 1, f.err
}

func (f *fakeUsers) get(_ context.Context, id string) (domain.User, error) {
	f.gotID = id

	return domain.User{ID: id}, f.err
}

func (f *fakeUsers) update(_ context.Context, id string, upd domain.UserUpdate) (domain.User, error) {
	f.gotID, f.gotUpdate = id, upd

	return domain.User{ID: id}, f.err
}

func (f *fakeUsers) delete(_ context.Context, id string) error {
	f.gotID = id

	return f.err
}

type (
	userCreatorFunc func(context.Context, domain.User) (domain.User, error)
	userListerFunc  func(context.Context, domain.UserFilter) ([]domain.User, int, error)
	userGetterFunc  func(context.Context, string) (domain.User, error)
	userUpdaterFunc func(context.Context, string, domain.UserUpdate) (domain.User, error)
	userDeleterFunc func(context.Context, string) error
)

func (f userCreatorFunc) Execute(ctx context.Context, u domain.User) (domain.User, error) {
	return f(ctx, u)
}

func (f userListerFunc) Execute(ctx context.Context, fl domain.UserFilter) ([]domain.User, int, error) {
	return f(ctx, fl)
}

func (f userGetterFunc) Execute(ctx context.Context, id string) (domain.User, error) {
	return f(ctx, id)
}

func (f userUpdaterFunc) Execute(ctx context.Context, id string, upd domain.UserUpdate) (domain.User, error) {
	return f(ctx, id, upd)
}

func (f userDeleterFunc) Execute(ctx context.Context, id string) error { return f(ctx, id) }

func TestUsersRequireAdminKey(t *testing.T) {
	s, _ := newTestServer(testAdminKey)

	if code, _ := do(t, s, http.MethodGet, "/v1/admin/users", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", code)
	}
}

func TestCreateUser(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	body := `{"name":"Dilnoza","phone_number":"+998901234567"}`
	if code, r := do(t, s, http.MethodPost, "/v1/admin/users", body, adminJSON); code != http.StatusOK || r.Status != _statusSuccess {
		t.Fatalf("create: %d %+v", code, r)
	}

	if got := d.users.gotUser; *got.Name != "Dilnoza" || *got.PhoneNumber != "+998901234567" || got.Username != nil {
		t.Fatalf("request not mapped: %+v", got)
	}

	if code, _ := do(t, s, http.MethodPost, "/v1/admin/users", "{broken", adminJSON); code != http.StatusBadRequest {
		t.Fatalf("broken json: %d", code)
	}

	d.users.err = errs.ErrPhoneNumberTaken
	if code, r := do(t, s, http.MethodPost, "/v1/admin/users", body, adminJSON); code != http.StatusConflict || r.ErrorCode != _errCodeConflict {
		t.Fatalf("conflict: %d %+v", code, r)
	}
}

func TestListUsers(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	code, r := do(t, s, http.MethodGet, "/v1/admin/users?limit=5&offset=10", "", adminJSON)
	if code != http.StatusOK || r.Status != _statusSuccess {
		t.Fatalf("list: %d %+v", code, r)
	}

	if want := (domain.UserFilter{Limit: 5, Offset: 10}); d.users.gotFilter != want {
		t.Fatalf("filter = %+v, want %+v", d.users.gotFilter, want)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/admin/users?limit=0", "", adminJSON); code != http.StatusBadRequest {
		t.Fatalf("bad limit: %d", code)
	}
}

func TestGetUser(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	if code, _ := do(t, s, http.MethodGet, "/v1/admin/users/"+testID, "", adminJSON); code != http.StatusOK || d.users.gotID != testID {
		t.Fatalf("get: %d id=%q", code, d.users.gotID)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/admin/users/not-a-uuid", "", adminJSON); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}

	d.users.err = errs.ErrUserNotFound
	if code, r := do(t, s, http.MethodGet, "/v1/admin/users/"+testID, "", adminJSON); code != http.StatusNotFound || r.ErrorCode != _errCodeNotFound {
		t.Fatalf("not found: %d %+v", code, r)
	}
}

func TestUpdateUser(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	body := `{"username":"","name":null}`
	if code, r := do(t, s, http.MethodPatch, "/v1/admin/users/"+testID, body, adminJSON); code != http.StatusOK {
		t.Fatalf("update: %d %+v", code, r)
	}

	// "" means clear, null and omitted mean keep.
	upd := d.users.gotUpdate
	if upd.Username == nil || *upd.Username != "" || upd.Name != nil || upd.PhoneNumber != nil {
		t.Fatalf("update not mapped: %+v", upd)
	}

	if code, _ := do(t, s, http.MethodPatch, "/v1/admin/users/not-a-uuid", body, adminJSON); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}

	d.users.err = errs.Errf(errs.ErrValidation, "nothing to update")
	if code, r := do(t, s, http.MethodPatch, "/v1/admin/users/"+testID, "{}", adminJSON); code != http.StatusUnprocessableEntity {
		t.Fatalf("validation: %d %+v", code, r)
	}
}

func TestDeleteUser(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	if code, r := do(t, s, http.MethodDelete, "/v1/admin/users/"+testID, "", adminJSON); code != http.StatusOK || r.Data != nil {
		t.Fatalf("delete: %d %+v", code, r)
	}

	d.users.err = errs.ErrUserNotFound
	if code, _ := do(t, s, http.MethodDelete, "/v1/admin/users/"+testID, "", adminJSON); code != http.StatusNotFound {
		t.Fatalf("not found: %d", code)
	}
}
