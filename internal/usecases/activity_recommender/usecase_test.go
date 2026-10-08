package activityrecommender

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeChildren struct {
	age int
	err error
}

func (f fakeChildren) GetForParent(_ context.Context, id, _ string) (domain.Child, error) {
	return domain.Child{ID: id, Age: f.age}, f.err
}

type fakeUsers struct {
	goalIDs []string
	called  bool
}

func (f *fakeUsers) Get(_ context.Context, id string) (domain.User, error) {
	f.called = true

	return domain.User{ID: id, GoalIDs: f.goalIDs}, nil
}

const goalID = "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"

type fakeActivities struct {
	got    domain.RecommendationQuery
	called bool
}

func (f *fakeActivities) Recommend(_ context.Context, q domain.RecommendationQuery) (domain.Activity, error) {
	f.got, f.called = q, true

	return domain.Activity{ID: "a"}, nil
}

func TestExecute(t *testing.T) {
	for age, want := range map[int]int{1: 2, 4: 4, 9: 6} {
		acts := &fakeActivities{}
		users := &fakeUsers{goalIDs: []string{goalID}}
		uc := New(nil, fakeChildren{age: age}, users, acts)
		uc.now = func() time.Time { return time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC) }

		if _, err := uc.Execute(context.Background(), Request{ChildID: "c", GoalID: goalID, MaxMinutes: 10}); err != nil {
			t.Fatalf("age %d: %v", age, err)
		}

		q := acts.got
		if q.Age != want || q.ChildID != "c" || q.GoalID != goalID || q.MaxMinutes != 10 || q.Day.Format(time.DateOnly) != "2026-10-03" {
			t.Errorf("age %d: query %+v", age, q)
		}

		// An explicit goal wins, so the caregiver's own goals are not looked up.
		if users.called || q.PreferGoalIDs != nil {
			t.Errorf("age %d: users called=%v prefer=%v", age, users.called, q.PreferGoalIDs)
		}
	}
}

func TestExecute_PrefersCaregiverGoals(t *testing.T) {
	acts := &fakeActivities{}
	users := &fakeUsers{goalIDs: []string{goalID}}

	if _, err := New(nil, fakeChildren{age: 3}, users, acts).Execute(context.Background(), Request{UserID: "u", ChildID: "c"}); err != nil {
		t.Fatal(err)
	}

	if acts.got.GoalID != "" || len(acts.got.PreferGoalIDs) != 1 || acts.got.PreferGoalIDs[0] != goalID {
		t.Errorf("query %+v", acts.got)
	}
}

func TestExecute_Rejected(t *testing.T) {
	cases := map[string]struct {
		children fakeChildren
		req      Request
		want     error
	}{
		"bad goal id":      {fakeChildren{age: 3}, Request{GoalID: "motor"}, errs.ErrValidation},
		"negative minutes": {fakeChildren{age: 3}, Request{MaxMinutes: -5}, errs.ErrValidation},
		"not my child":     {fakeChildren{err: errs.ErrChildNotFound}, Request{}, errs.ErrChildNotFound},
	}

	for name, tc := range cases {
		acts := &fakeActivities{}
		if _, err := New(nil, tc.children, &fakeUsers{}, acts).Execute(context.Background(), tc.req); !errors.Is(err, tc.want) || acts.called {
			t.Errorf("%s: err=%v called=%v", name, err, acts.called)
		}
	}
}
