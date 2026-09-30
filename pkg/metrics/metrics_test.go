package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scrape(t *testing.T) string {
	t.Helper()

	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	return w.Body.String()
}

func TestSQLOperation(t *testing.T) {
	for sql, want := range map[string]string{
		"SELECT 1":                    "select",
		"\n\t insert into x values()": "insert",
		"with a as (select 1) select": "with",
		"BEGIN;":                      "begin",
		"   ":                         "unknown",
	} {
		if got := sqlOperation(sql); got != want {
			t.Errorf("sqlOperation(%q) = %q, want %q", sql, got, want)
		}
	}
}

func TestRoundTripper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	client := &http.Client{Transport: RoundTripper("test_svc", nil)}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()

	want := `birga_http_client_request_duration_seconds_count{method="GET",service="test_svc",status="418"} 1`
	if out := scrape(t); !strings.Contains(out, want) {
		t.Fatalf("missing %s", want)
	}
}

func TestRuntimeCollectorsRegistered(t *testing.T) {
	out := scrape(t)
	for _, name := range []string{"go_goroutines", "process_cpu_seconds_total"} {
		if !strings.Contains(out, name) {
			t.Errorf("missing %s", name)
		}
	}
}
