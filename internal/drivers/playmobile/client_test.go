package playmobile

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func newClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return New(nil, &config.PlayMobileConfig{
		BaseURL: srv.URL, Username: "user", Password: "pass", Originator: "3700", Timeout: time.Second,
	})
}

func TestSend_Success(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if r.Method != http.MethodPost || r.URL.Path != "/"+pathSend || !ok || user != "user" || pass != "pass" {
			t.Errorf("unexpected request: %s %s auth=%v %q %q", r.Method, r.URL.Path, ok, user, pass)
		}

		var req sendReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 1 {
			t.Fatalf("bad body: %+v, %v", req, err)
		}

		m := req.Messages[0]
		if m.Recipient != "998901234567" || m.MessageID != "id-1" || m.SMS.Originator != "3700" || m.SMS.Content.Text != "hi" {
			t.Errorf("unexpected message: %+v", m)
		}

		// Play Mobile answers 200 with a plain-text body.
		_, _ = w.Write([]byte("Request is received"))
	})

	if err := c.Send(context.Background(), "id-1", "+998901234567", "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSend_Rejected(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error-code": 102, "error-description": "Account is locked"}`))
	})

	err := c.Send(context.Background(), "id-1", "+998901234567", "hi")
	if !errors.Is(err, errs.ErrInternal) || err.Error() != "play mobile: status 400, error 102: Account is locked" {
		t.Fatalf("expected internal error with provider details, got %v", err)
	}
}

func TestSend_ServerError(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	if err := c.Send(context.Background(), "id-1", "+998901234567", "hi"); !errors.Is(err, errs.ErrConnection) {
		t.Fatalf("expected connection error, got %v", err)
	}
}
