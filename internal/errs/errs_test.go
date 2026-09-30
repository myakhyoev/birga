package errs

import (
	"errors"
	"testing"
)

func TestErrfMatchesParent(t *testing.T) {
	err := Errf(ErrNotFound, "user %d not found", 7)

	if !errors.Is(err, ErrNotFound) {
		t.Fatal("expected errors.Is(err, ErrNotFound)")
	}

	if err.Error() != "user 7 not found" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestNestedSentinel(t *testing.T) {
	if !errors.Is(ErrActivityNotFound, ErrNotFound) {
		t.Fatal("ErrActivityNotFound should match ErrNotFound")
	}
}

func TestWrap(t *testing.T) {
	if Wrap(nil) != nil {
		t.Fatal("Wrap(nil) should be nil")
	}

	plain := errors.New("boom")
	if !errors.Is(Wrap(plain), ErrInternal) {
		t.Fatal("plain error should be wrapped as ErrInternal")
	}

	app := Errf(ErrValidation, "bad")
	if got := Wrap(app); !errors.Is(got, ErrValidation) || errors.Is(got, ErrInternal) {
		t.Fatal("application error should pass through unchanged")
	}
}
