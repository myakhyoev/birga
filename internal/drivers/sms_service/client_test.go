package smsservice

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

	return New(nil, &config.SMSServiceConfig{BaseURL: srv.URL, Token: "tkn", From: "4546", Timeout: time.Second})
}

func TestSend_Success(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+pathSend || r.Header.Get("Authorization") != "Bearer tkn" {
			t.Errorf("unexpected request: %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}

		var req sendReq
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.MobilePhone != "998901234567" || req.From != "4546" || req.Message != "hi" {
			t.Errorf("unexpected body: %+v", req)
		}

		_ = json.NewEncoder(w).Encode(sendResp{ID: "msg-1", Status: "waiting"})
	})

	id, err := c.Send(context.Background(), "998901234567", "hi")
	if err != nil || id != "msg-1" {
		t.Fatalf("got %q, %v", id, err)
	}
}

func TestSend_ClientError(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	})

	if _, err := c.Send(context.Background(), "1", "hi"); !errors.Is(err, errs.ErrBadRequest) {
		t.Fatalf("expected bad request, got %v", err)
	}
}

func TestSend_ServerError(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	if _, err := c.Send(context.Background(), "1", "hi"); !errors.Is(err, errs.ErrConnection) {
		t.Fatalf("expected connection error, got %v", err)
	}
}

func TestSend_EmptyID(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(sendResp{Status: "failed"})
	})

	if _, err := c.Send(context.Background(), "1", "hi"); !errors.Is(err, errs.ErrInternal) {
		t.Fatalf("expected internal error, got %v", err)
	}
}
