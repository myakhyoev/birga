package userlogout

import (
	"context"
	"testing"
)

type fakeAuth struct{ userID, access, refresh string }

func (f *fakeAuth) SetTokens(_ context.Context, userID, access, refresh string) error {
	f.userID, f.access, f.refresh = userID, access, refresh

	return nil
}

func TestExecute(t *testing.T) {
	a := &fakeAuth{access: "x", refresh: "y"}

	if err := New(nil, a).Execute(context.Background(), "u1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if a.userID != "u1" || a.access != "" || a.refresh != "" {
		t.Fatalf("tokens not cleared: %+v", a)
	}
}
