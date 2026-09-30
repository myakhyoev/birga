package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type echo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query"`
	Header string `json:"header"`
	Body   string `json:"body"`
}

func newEchoServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"upstream"}`))

			return
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)

		_ = json.NewEncoder(w).Encode(echo{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Header: r.Header.Get("X-Api-Key"),
			Body:   string(raw),
		})
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestGet(t *testing.T) {
	srv := newEchoServer(t)
	c := New(srv.URL+"/", WithHeader("X-Api-Key", "secret"))

	var got echo
	if err := c.Get(context.Background(), &got, "/items", url.Values{"a": {"1"}}, nil); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/items" || got.Query != "a=1" || got.Header != "secret" {
		t.Fatalf("unexpected echo: %+v", got)
	}
}

func TestPost(t *testing.T) {
	srv := newEchoServer(t)
	c := New(srv.URL)

	var got echo
	if err := c.Post(context.Background(), &got, "items", map[string]string{"k": "v"}, map[string]string{"X-Api-Key": "h"}); err != nil {
		t.Fatalf("Post: %v", err)
	}

	if got.Method != http.MethodPost || got.Body != `{"k":"v"}` || got.Header != "h" {
		t.Fatalf("unexpected echo: %+v", got)
	}
}

func TestNilDestDiscardsBody(t *testing.T) {
	srv := newEchoServer(t)

	if err := New(srv.URL).Delete(context.Background(), nil, "items/1", nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestStatusError(t *testing.T) {
	srv := newEchoServer(t)

	err := New(srv.URL).Get(context.Background(), nil, "fail", nil, nil)

	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("expected *StatusError, got %v", err)
	}

	if se.StatusCode != http.StatusBadGateway || string(se.Body) != `{"error":"upstream"}` {
		t.Fatalf("unexpected status error: %+v", se)
	}
}
