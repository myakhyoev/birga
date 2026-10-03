package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
)

const (
	testAdminKey = "test-key"
	testID       = "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"
)

var adminJSON = map[string]string{adminKeyHeader: testAdminKey, "Content-Type": "application/json"}

type fakeHealth struct{ err error }

func (f *fakeHealth) Ping(context.Context) error { return f.err }

type fakeCreator struct {
	got domain.Activity
	err error
}

func (f *fakeCreator) Execute(_ context.Context, a domain.Activity) (domain.Activity, error) {
	f.got = a
	a.ID = testID

	return a, f.err
}

type fakeLister struct {
	got domain.ActivityFilter
	err error
}

func (f *fakeLister) Execute(_ context.Context, fl domain.ActivityFilter) ([]domain.Activity, int, error) {
	f.got = fl

	return []domain.Activity{{ID: testID}}, 1, f.err
}

type fakeGetter struct {
	gotUnpublished bool
	err            error
}

func (f *fakeGetter) Execute(_ context.Context, id string, includeUnpublished bool) (domain.Activity, error) {
	f.gotUnpublished = includeUnpublished

	return domain.Activity{ID: id}, f.err
}

type deps struct {
	health  *fakeHealth
	creator *fakeCreator
	lister  *fakeLister
	getter  *fakeGetter
	users   *fakeUsers
	otp     *fakeOTPSender
	media   *fakeMedia
	auth    *fakeAuth
	edit    *fakeActivityEdit
	kids    *fakeChildActivities
}

func newTestServer(adminKey string) (*Server, *deps) {
	gin.SetMode(gin.TestMode)

	d := &deps{health: &fakeHealth{}, creator: &fakeCreator{}, lister: &fakeLister{}, getter: &fakeGetter{}, users: &fakeUsers{}, otp: &fakeOTPSender{}, media: &fakeMedia{}, auth: &fakeAuth{},
		edit: &fakeActivityEdit{}, kids: &fakeChildActivities{}}
	s := New(config.Application{AdminAPIKey: adminKey}, nil, d.health, d.creator, d.lister, d.getter,
		ActivityEditUseCases{Updater: d.edit, Deleter: activityDeleterFunc(d.edit.delete)},
		ChildActivityUseCases{
			Creator:     childCreatorFunc(d.kids.create),
			Recommender: d.kids,
			Recorder:    recorderFunc(d.kids.record),
			Lister:      completionListerFunc(d.kids.list),
			Streak:      streakFunc(d.kids.streak),
		}, UserUseCases{
			Creator: userCreatorFunc(d.users.create),
			Lister:  userListerFunc(d.users.list),
			Getter:  userGetterFunc(d.users.get),
			Updater: userUpdaterFunc(d.users.update),
			Deleter: userDeleterFunc(d.users.delete),
		}, OTPUseCases{Sender: d.otp, Verifier: otpVerifierFunc(d.otp.verify)},
		AuthUseCases{SignUp: d.auth, Refresher: refresherFunc(d.auth.refresh), Checker: checkerFunc(d.auth.check)}, d.media)

	return s, d
}

func do(t *testing.T, s http.Handler, method, path, body string, headers map[string]string) (int, R) {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var r R
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("response is not JSON envelope: %q", w.Body.String())
	}

	return w.Code, r
}

func TestPingAndHealth(t *testing.T) {
	s, d := newTestServer("")

	if code, r := do(t, s, http.MethodGet, "/ping", "", nil); code != http.StatusOK || r.Data != "Pong" {
		t.Fatalf("ping: %d %+v", code, r)
	}

	if code, _ := do(t, s, http.MethodGet, "/health", "", nil); code != http.StatusOK {
		t.Fatalf("health: %d", code)
	}

	d.health.err = errors.New("db down")

	if code, r := do(t, s, http.MethodGet, "/health", "", nil); code != http.StatusServiceUnavailable || r.ErrorCode != _errCodeUnavailable {
		t.Fatalf("health failing: %d %+v", code, r)
	}
}

func TestListActivities(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodGet, "/v1/activities?age=4&goal=motor&limit=500&offset=10", "", nil)
	if code != http.StatusOK || r.Status != _statusSuccess {
		t.Fatalf("got %d %+v", code, r)
	}

	want := domain.ActivityFilter{Age: 4, Goal: "motor", PublishedOnly: true, Limit: maxPageLimit, Offset: 10}
	if d.lister.got != want {
		t.Fatalf("filter = %+v, want %+v", d.lister.got, want)
	}
}

func TestListActivities_BadQuery(t *testing.T) {
	s, _ := newTestServer("")

	for _, q := range []string{"limit=0", "limit=x", "offset=-1", "age=four"} {
		if code, r := do(t, s, http.MethodGet, "/v1/activities?"+q, "", nil); code != http.StatusBadRequest || r.ErrorCode != _errCodeBadRequest {
			t.Fatalf("%s: got %d %+v", q, code, r)
		}
	}
}

func TestGetActivity(t *testing.T) {
	s, d := newTestServer("")

	if code, _ := do(t, s, http.MethodGet, "/v1/activities/"+testID, "", nil); code != http.StatusOK || d.getter.gotUnpublished {
		t.Fatalf("public get: %d unpublished=%v", code, d.getter.gotUnpublished)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/activities/not-a-uuid", "", nil); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}

	d.getter.err = errs.ErrActivityNotFound

	if code, r := do(t, s, http.MethodGet, "/v1/activities/"+testID, "", nil); code != http.StatusNotFound || r.ErrorCode != _errCodeNotFound {
		t.Fatalf("not found: %d %+v", code, r)
	}
}

func TestAdminAuth(t *testing.T) {
	disabled, _ := newTestServer("")
	if code, _ := do(t, disabled, http.MethodGet, "/v1/admin/activities", "", map[string]string{adminKeyHeader: "x"}); code != http.StatusForbidden {
		t.Fatalf("disabled admin: %d", code)
	}

	s, d := newTestServer(testAdminKey)
	if code, _ := do(t, s, http.MethodGet, "/v1/admin/activities", "", map[string]string{adminKeyHeader: "wrong"}); code != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", code)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/admin/activities", "", map[string]string{adminKeyHeader: testAdminKey}); code != http.StatusOK || d.lister.got.PublishedOnly {
		t.Fatalf("admin list: %d publishedOnly=%v", code, d.lister.got.PublishedOnly)
	}
}

func TestCreateActivity(t *testing.T) {
	s, d := newTestServer(testAdminKey)
	auth := adminJSON

	body := `{"title_uz":"T","title_ru":"Т","description_uz":"D","description_ru":"Д","goal":"motor","min_age":2,"max_age":4,"duration_minutes":5}`
	if code, r := do(t, s, http.MethodPost, "/v1/admin/activities", body, auth); code != http.StatusOK || r.Status != _statusSuccess {
		t.Fatalf("create: %d %+v", code, r)
	}

	if d.creator.got.Goal != "motor" || d.creator.got.MaxAge != 4 {
		t.Fatalf("request not mapped: %+v", d.creator.got)
	}

	if code, _ := do(t, s, http.MethodPost, "/v1/admin/activities", "{broken", auth); code != http.StatusBadRequest {
		t.Fatalf("broken json: %d", code)
	}

	d.creator.err = errs.Errf(errs.ErrValidation, "unknown goal")
	if code, r := do(t, s, http.MethodPost, "/v1/admin/activities", body, auth); code != http.StatusUnprocessableEntity || r.ErrorCode != _errCodeValidation {
		t.Fatalf("validation: %d %+v", code, r)
	}
}

func TestInternalErrorIsNotLeaked(t *testing.T) {
	s, d := newTestServer("")
	d.lister.err = errors.New(`pq: relation "activities" does not exist`)

	code, r := do(t, s, http.MethodGet, "/v1/activities", "", nil)
	if code != http.StatusInternalServerError || r.ErrorNote != errs.ErrInternal.Error() {
		t.Fatalf("got %d %+v", code, r)
	}
}

func TestRequestIDAndMetrics(t *testing.T) {
	s, _ := newTestServer("")

	req := httptest.NewRequest(http.MethodGet, "/v1/activities/"+testID, nil)
	req.Header.Set("X-Request-Id", "req-123")

	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("X-Request-Id = %q", got)
	}

	mw := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(mw, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	want := `birga_http_server_request_duration_seconds_count{code="0",method="GET",route="/v1/activities/:id",status="200"}`
	if !strings.Contains(mw.Body.String(), want) {
		t.Fatalf("metrics output missing %s", want)
	}
}

type fakeActivityEdit struct {
	gotID     string
	got       domain.ActivityUpdate
	deletedID string
	err       error
}

func (f *fakeActivityEdit) Execute(_ context.Context, id string, upd domain.ActivityUpdate) (domain.Activity, error) {
	f.gotID, f.got = id, upd

	return domain.Activity{ID: id}, f.err
}

func (f *fakeActivityEdit) delete(id string) error {
	f.deletedID = id

	return f.err
}

type activityDeleterFunc func(string) error

func (fn activityDeleterFunc) Execute(_ context.Context, id string) error { return fn(id) }

func TestUpdateActivity(t *testing.T) {
	s, d := newTestServer(testAdminKey)
	path := "/v1/admin/activities/" + testID

	if code, r := do(t, s, http.MethodPatch, path, `{"is_published":true,"min_age":3}`, adminJSON); code != http.StatusOK {
		t.Fatalf("update: %d %+v", code, r)
	}

	if d.edit.gotID != testID || d.edit.got.IsPublished == nil || !*d.edit.got.IsPublished ||
		d.edit.got.MinAge == nil || *d.edit.got.MinAge != 3 || d.edit.got.TitleUz != nil {
		t.Fatalf("update not mapped: %+v", d.edit.got)
	}

	if code, _ := do(t, s, http.MethodPatch, "/v1/admin/activities/x", `{}`, adminJSON); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}

	if code, _ := do(t, s, http.MethodPatch, path, `{"min_age":"three"}`, adminJSON); code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", code)
	}

	d.edit.err = errs.ErrActivityNotFound

	if code, _ := do(t, s, http.MethodPatch, path, `{"is_published":false}`, adminJSON); code != http.StatusNotFound {
		t.Fatalf("not found: %d", code)
	}
}

func TestDeleteActivity(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	if code, r := do(t, s, http.MethodDelete, "/v1/admin/activities/"+testID, "", adminJSON); code != http.StatusOK || d.edit.deletedID != testID {
		t.Fatalf("delete: %d %+v", code, r)
	}

	if code, _ := do(t, s, http.MethodDelete, "/v1/admin/activities/"+testID, "", nil); code != http.StatusUnauthorized {
		t.Fatalf("delete without key: %d", code)
	}
}
