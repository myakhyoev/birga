package s3storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c, err := New(context.Background(), zap.NewNop(), &config.S3Config{
		Bucket: "birga-test", Region: "eu-central-1", AccessKeyID: "AKID", SecretAccessKey: "secret",
		Endpoint: srv.URL, Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return c
}

func TestPut(t *testing.T) {
	var (
		gotMethod, gotPath, gotType, gotAuth string
		gotBody                              []byte
	)

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotType, gotAuth = r.Method, r.URL.Path, r.Header.Get("Content-Type"), r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
	})

	if err := c.Put(context.Background(), "media/a.png", "image/png", []byte("png-bytes")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if gotMethod != http.MethodPut || gotPath != "/birga-test/media/a.png" || gotType != "image/png" {
		t.Fatalf("got %s %s %s", gotMethod, gotPath, gotType)
	}

	if string(gotBody) != "png-bytes" || gotAuth == "" {
		t.Fatalf("body %q, auth %q", gotBody, gotAuth)
	}
}

func TestPut_Error(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`))
	})

	if err := c.Put(context.Background(), "media/a.png", "image/png", []byte("x")); !errors.Is(err, errs.ErrConnection) {
		t.Fatalf("expected connection error, got %v", err)
	}
}

func TestPublicURL(t *testing.T) {
	cases := []struct {
		cfg  config.S3Config
		want string
	}{
		{config.S3Config{Bucket: "b", Region: "eu-central-1"}, "https://b.s3.eu-central-1.amazonaws.com/media/a b.png"},
		{config.S3Config{Bucket: "b", PublicBaseURL: "https://cdn.birga.uz/"}, "https://cdn.birga.uz/media/a b.png"},
		{config.S3Config{Bucket: "b", Endpoint: "http://localhost:9000"}, "http://localhost:9000/b/media/a b.png"},
	}

	for _, tc := range cases {
		c := &Client{baseURL: publicBaseURL(&tc.cfg)}

		want := tc.want[:len(tc.want)-len("a b.png")] + "a%20b.png"
		if got := c.URL("media/a b.png"); got != want {
			t.Errorf("URL = %q, want %q", got, want)
		}
	}
}
