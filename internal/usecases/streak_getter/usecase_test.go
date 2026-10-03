package streakgetter

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeChildren struct{ err error }

func (f fakeChildren) GetForParent(_ context.Context, id, _ string) (domain.Child, error) {
	return domain.Child{ID: id}, f.err
}

type fakeCompletions struct{ called bool }

func (f *fakeCompletions) Days(context.Context, string) ([]time.Time, int, error) {
	f.called = true

	return []time.Time{time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}, 3, nil
}

func TestExecute(t *testing.T) {
	uc := New(nil, fakeChildren{}, &fakeCompletions{})
	uc.now = func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }

	s, err := uc.Execute(context.Background(), "u", "c")
	if err != nil || s.Current != 2 || !s.CompletedToday || s.Total != 3 {
		t.Fatalf("streak: %+v %v", s, err)
	}

	comp := &fakeCompletions{}
	if _, err := New(nil, fakeChildren{err: errs.ErrChildNotFound}, comp).Execute(context.Background(), "u", "c"); !errors.Is(err, errs.ErrChildNotFound) || comp.called {
		t.Fatalf("not my child: %v called=%v", err, comp.called)
	}
}
