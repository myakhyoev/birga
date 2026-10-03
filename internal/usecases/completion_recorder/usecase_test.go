package completionrecorder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeChildren struct{ err error }

func (f fakeChildren) GetForParent(_ context.Context, id, _ string) (domain.Child, error) {
	return domain.Child{ID: id, Age: 4}, f.err
}

type fakeActivities struct {
	a   domain.Activity
	err error
}

func (f fakeActivities) Get(context.Context, string) (domain.Activity, error) { return f.a, f.err }

type fakeCompletions struct {
	got    domain.Completion
	called bool
}

func (f *fakeCompletions) Create(_ context.Context, c domain.Completion) (domain.Completion, bool, error) {
	f.got, f.called = c, true

	return c, true, nil
}

func newUC(children fakeChildren, activities fakeActivities, completions *fakeCompletions) *UseCase {
	uc := New(nil, children, activities, completions)
	uc.now = func() time.Time { return time.Date(2026, 10, 3, 21, 0, 0, 0, time.UTC) } // Oct 4 in Tashkent

	return uc
}

func TestExecute_Success(t *testing.T) {
	comp := &fakeCompletions{}
	note := "  yaxshi o'tdi  "

	_, err := newUC(fakeChildren{}, fakeActivities{a: domain.Activity{IsPublished: true}}, comp).
		Execute(context.Background(), Request{UserID: "u", ChildID: "c", ActivityID: "a", Note: &note})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if comp.got.CompletedOn.Format(time.DateOnly) != "2026-10-04" || *comp.got.Note != "yaxshi o'tdi" || *comp.got.UserID != "u" {
		t.Fatalf("unexpected completion: %+v", comp.got)
	}

	blank := "   "
	if _, err := newUC(fakeChildren{}, fakeActivities{a: domain.Activity{IsPublished: true}}, comp).
		Execute(context.Background(), Request{ChildID: "c", ActivityID: "a", Note: &blank}); err != nil || comp.got.Note != nil {
		t.Fatalf("blank note should be dropped: %v %+v", err, comp.got.Note)
	}
}

func TestExecute_Rejected(t *testing.T) {
	long := strings.Repeat("я", domain.MaxCompletionNoteLength+1)
	published := fakeActivities{a: domain.Activity{IsPublished: true}}

	cases := map[string]struct {
		children   fakeChildren
		activities fakeActivities
		note       *string
		want       error
	}{
		"not my child":  {fakeChildren{err: errs.ErrChildNotFound}, published, nil, errs.ErrChildNotFound},
		"no activity":   {fakeChildren{}, fakeActivities{err: errs.ErrActivityNotFound}, nil, errs.ErrActivityNotFound},
		"unpublished":   {fakeChildren{}, fakeActivities{}, nil, errs.ErrActivityNotFound},
		"note too long": {fakeChildren{}, published, &long, errs.ErrValidation},
	}

	for name, tc := range cases {
		comp := &fakeCompletions{}

		_, err := newUC(tc.children, tc.activities, comp).
			Execute(context.Background(), Request{ChildID: "c", ActivityID: "a", Note: tc.note})
		if !errors.Is(err, tc.want) || comp.called {
			t.Errorf("%s: err=%v called=%v", name, err, comp.called)
		}
	}
}
