package rest

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	activityrecommender "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_recommender"
	completionrecorder "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_recorder"
)

const (
	testUserID  = "9a8b7c6d-5e4f-4a3b-2c1d-0e9f8a7b6c5d"
	testChildID = "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"
)

var bearer = map[string]string{authorizationHeader: "Bearer good"}

type fakeChildActivities struct {
	gotRecommend activityrecommender.Request
	gotRecord    completionrecorder.Request
	gotList      domain.CompletionFilter
	gotUserID    string
	err          error
}

func (f *fakeChildActivities) Execute(_ context.Context, req activityrecommender.Request) (domain.Activity, error) {
	f.gotRecommend = req

	return domain.Activity{ID: testID}, f.err
}

func (f *fakeChildActivities) record(req completionrecorder.Request) (domain.Completion, error) {
	f.gotRecord = req

	return domain.Completion{ID: testID, ChildID: req.ChildID, ActivityID: req.ActivityID,
		CompletedOn: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), Note: req.Note}, f.err
}

func (f *fakeChildActivities) list(userID string, fl domain.CompletionFilter) ([]domain.Completion, int, error) {
	f.gotUserID, f.gotList = userID, fl

	return []domain.Completion{{ID: testID}}, 1, f.err
}

func (f *fakeChildActivities) streak(userID, childID string) (domain.Streak, error) {
	f.gotUserID = userID
	last := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)

	return domain.Streak{Current: 2, Longest: 5, CompletedToday: true, ThisWeek: 2, Total: 9, LastCompletedOn: &last}, f.err
}

type recorderFunc func(completionrecorder.Request) (domain.Completion, error)

func (fn recorderFunc) Execute(_ context.Context, req completionrecorder.Request) (domain.Completion, error) {
	return fn(req)
}

type completionListerFunc func(string, domain.CompletionFilter) ([]domain.Completion, int, error)

func (fn completionListerFunc) Execute(_ context.Context, userID string, f domain.CompletionFilter) ([]domain.Completion, int, error) {
	return fn(userID, f)
}

type streakFunc func(string, string) (domain.Streak, error)

func (fn streakFunc) Execute(_ context.Context, userID, childID string) (domain.Streak, error) {
	return fn(userID, childID)
}

func TestUserAuth(t *testing.T) {
	s, _ := newTestServer("")
	path := "/v1/children/" + testChildID + "/streak"

	for name, h := range map[string]map[string]string{
		"missing": nil,
		"basic":   {authorizationHeader: "Basic abc"},
		"invalid": {authorizationHeader: "Bearer bad"},
	} {
		if code, r := do(t, s, http.MethodGet, path, "", h); code != http.StatusUnauthorized || r.ErrorCode != _errCodeUnauthorized {
			t.Fatalf("%s: got %d %+v", name, code, r)
		}
	}
}

func TestRecommendActivity(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodGet, "/v1/children/"+testChildID+"/recommendation?goal=motor&minutes=10", "", bearer)
	if code != http.StatusOK {
		t.Fatalf("got %d %+v", code, r)
	}

	want := activityrecommender.Request{UserID: testUserID, ChildID: testChildID, Goal: "motor", MaxMinutes: 10}
	if d.kids.gotRecommend != want {
		t.Fatalf("request = %+v, want %+v", d.kids.gotRecommend, want)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/children/"+testChildID+"/recommendation?minutes=ten", "", bearer); code != http.StatusBadRequest {
		t.Fatalf("bad minutes: %d", code)
	}

	if code, _ := do(t, s, http.MethodGet, "/v1/children/nope/recommendation", "", bearer); code != http.StatusBadRequest {
		t.Fatalf("bad child id: %d", code)
	}

	d.kids.err = errs.ErrNoRecommendation

	if code, _ := do(t, s, http.MethodGet, "/v1/children/"+testChildID+"/recommendation", "", bearer); code != http.StatusNotFound {
		t.Fatalf("no recommendation: %d", code)
	}
}

func TestCompleteActivity(t *testing.T) {
	s, d := newTestServer("")
	path := "/v1/children/" + testChildID + "/completions"

	code, r := do(t, s, http.MethodPost, path, `{"activity_id":"`+testID+`","note":"zo'r"}`, bearer)
	if code != http.StatusOK {
		t.Fatalf("got %d %+v", code, r)
	}

	got := d.kids.gotRecord
	if got.UserID != testUserID || got.ChildID != testChildID || got.ActivityID != testID || got.Note == nil || *got.Note != "zo'r" {
		t.Fatalf("request not mapped: %+v", got)
	}

	if data, _ := r.Data.(map[string]any); data["completed_on"] != "2026-10-03" {
		t.Fatalf("completed_on = %v", data["completed_on"])
	}

	if code, _ := do(t, s, http.MethodPost, path, `{"activity_id":"x"}`, bearer); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad activity id: %d", code)
	}

	if code, _ := do(t, s, http.MethodPost, path, `{broken`, bearer); code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", code)
	}
}

func TestListCompletionsAndStreak(t *testing.T) {
	s, d := newTestServer("")
	base := "/v1/children/" + testChildID

	if code, r := do(t, s, http.MethodGet, base+"/completions?limit=5", "", bearer); code != http.StatusOK {
		t.Fatalf("list: %d %+v", code, r)
	}

	want := domain.CompletionFilter{ChildID: testChildID, Limit: 5}
	if d.kids.gotList != want || d.kids.gotUserID != testUserID {
		t.Fatalf("filter = %+v user %q", d.kids.gotList, d.kids.gotUserID)
	}

	code, r := do(t, s, http.MethodGet, base+"/streak", "", bearer)
	if code != http.StatusOK {
		t.Fatalf("streak: %d %+v", code, r)
	}

	if data, _ := r.Data.(map[string]any); data["current"] != float64(2) || data["last_completed_on"] != "2026-10-03" {
		t.Fatalf("streak view: %+v", r.Data)
	}

	d.kids.err = errs.ErrChildNotFound

	if code, _ := do(t, s, http.MethodGet, base+"/streak", "", bearer); code != http.StatusNotFound {
		t.Fatalf("not my child: %d", code)
	}
}
