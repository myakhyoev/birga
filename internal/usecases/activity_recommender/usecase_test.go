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
		uc := New(nil, fakeChildren{age: age}, acts)
		uc.now = func() time.Time { return time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC) }

		if _, err := uc.Execute(context.Background(), Request{ChildID: "c", Goal: "motor", MaxMinutes: 10}); err != nil {
			t.Fatalf("age %d: %v", age, err)
		}

		q := acts.got
		if q.Age != want || q.ChildID != "c" || q.Goal != "motor" || q.MaxMinutes != 10 || q.Day.Format(time.DateOnly) != "2026-10-03" {
			t.Errorf("age %d: query %+v", age, q)
		}
	}
}

func TestExecute_Rejected(t *testing.T) {
	cases := map[string]struct {
		children fakeChildren
		req      Request
		want     error
	}{
		"unknown goal":     {fakeChildren{age: 3}, Request{Goal: "flying"}, errs.ErrValidation},
		"negative minutes": {fakeChildren{age: 3}, Request{MaxMinutes: -5}, errs.ErrValidation},
		"not my child":     {fakeChildren{err: errs.ErrChildNotFound}, Request{}, errs.ErrChildNotFound},
	}

	for name, tc := range cases {
		acts := &fakeActivities{}
		if _, err := New(nil, tc.children, acts).Execute(context.Background(), tc.req); !errors.Is(err, tc.want) || acts.called {
			t.Errorf("%s: err=%v called=%v", name, err, acts.called)
		}
	}
}
