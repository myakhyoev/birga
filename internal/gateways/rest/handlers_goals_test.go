package rest

import (
	"context"
	"net/http"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeGoals struct {
	got       domain.Goal
	gotUpdate domain.GoalUpdate
	gotID     string
	err       error
}

func (f *fakeGoals) create(g domain.Goal) (domain.Goal, error) {
	f.got = g
	g.ID = testID

	return g, f.err
}

func (f *fakeGoals) list() ([]domain.Goal, error) {
	return []domain.Goal{{ID: testID, NameEn: "Speech"}}, f.err
}

func (f *fakeGoals) get(id string) (domain.Goal, error) {
	f.gotID = id

	return domain.Goal{ID: id}, f.err
}

func (f *fakeGoals) update(id string, upd domain.GoalUpdate) (domain.Goal, error) {
	f.gotID, f.gotUpdate = id, upd

	return domain.Goal{ID: id}, f.err
}

func (f *fakeGoals) delete(id string) error {
	f.gotID = id

	return f.err
}

type goalCreatorFunc func(domain.Goal) (domain.Goal, error)

func (fn goalCreatorFunc) Execute(_ context.Context, g domain.Goal) (domain.Goal, error) {
	return fn(g)
}

type goalListerFunc func() ([]domain.Goal, error)

func (fn goalListerFunc) Execute(context.Context) ([]domain.Goal, error) { return fn() }

type goalGetterFunc func(string) (domain.Goal, error)

func (fn goalGetterFunc) Execute(_ context.Context, id string) (domain.Goal, error) { return fn(id) }

type goalUpdaterFunc func(string, domain.GoalUpdate) (domain.Goal, error)

func (fn goalUpdaterFunc) Execute(_ context.Context, id string, upd domain.GoalUpdate) (domain.Goal, error) {
	return fn(id, upd)
}

type goalDeleterFunc func(string) error

func (fn goalDeleterFunc) Execute(_ context.Context, id string) error { return fn(id) }

func TestListAndGetGoals_NoToken(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodGet, "/v1/goals", "", nil)
	data, _ := r.Data.(map[string]any)
	items, _ := data["items"].([]any)

	if code != http.StatusOK || len(items) != 1 || data["total"] != float64(1) {
		t.Fatalf("list: %d %+v", code, r)
	}

	if code, r := do(t, s, http.MethodGet, "/v1/goals/"+testID, "", nil); code != http.StatusOK || d.goals.gotID != testID {
		t.Fatalf("get: %d %+v", code, r)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/goals/nope", "", nil); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}

	d.goals.err = errs.ErrGoalNotFound

	if code, _ := do(t, s, http.MethodGet, "/v1/goals/"+testID, "", nil); code != http.StatusNotFound {
		t.Fatalf("missing: %d", code)
	}
}

func TestGoalAdminEndpoints(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	code, r := do(t, s, http.MethodPost, "/v1/admin/goals", `{"name_uz": "Nutq", "name_ru": "Речь", "name_en": "Speech"}`, adminJSON)
	if code != http.StatusOK || d.goals.got != (domain.Goal{NameUz: "Nutq", NameRu: "Речь", NameEn: "Speech"}) {
		t.Fatalf("create: %d %+v, got %+v", code, r, d.goals.got)
	}

	code, r = do(t, s, http.MethodPatch, "/v1/admin/goals/"+testID, `{"name_en": "Talking"}`, adminJSON)
	if code != http.StatusOK || d.goals.gotID != testID || *d.goals.gotUpdate.NameEn != "Talking" || d.goals.gotUpdate.NameUz != nil {
		t.Fatalf("update: %d %+v", code, r)
	}

	if code, r := do(t, s, http.MethodDelete, "/v1/admin/goals/"+testID, "", adminJSON); code != http.StatusOK {
		t.Fatalf("delete: %d %+v", code, r)
	}

	d.goals.err = errs.ErrGoalNameTaken

	if code, _ := do(t, s, http.MethodPost, "/v1/admin/goals", `{}`, adminJSON); code != http.StatusConflict {
		t.Fatalf("taken: %d", code)
	}
}

func TestGoalAdminEndpoints_Auth(t *testing.T) {
	s, _ := newTestServer(testAdminKey)

	if code, _ := do(t, s, http.MethodPost, "/v1/admin/goals", `{}`, nil); code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", code)
	}

	user := map[string]string{authorizationHeader: "Bearer good"}
	if code, _ := do(t, s, http.MethodDelete, "/v1/admin/goals/"+testID, "", user); code != http.StatusForbidden {
		t.Fatalf("regular user: %d", code)
	}

	admin := map[string]string{authorizationHeader: "Bearer admin"}
	if code, _ := do(t, s, http.MethodDelete, "/v1/admin/goals/"+testID, "", admin); code != http.StatusOK {
		t.Fatalf("admin user: %d", code)
	}
}
