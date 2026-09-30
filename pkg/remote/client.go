// Package remote is a small JSON-over-HTTP client for calling other services.
//
// Every call takes a context, a destination to decode the response body
// into (nil to discard it) and a path relative to the base URL. Non-2xx
// responses are returned as *StatusError so callers can map them.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout   = 15 * time.Second
	maxErrorBodySize = 64 << 10
)

// Client is the interface drivers depend on; the concrete *HTTPClient implements it.
type Client interface {
	Get(ctx context.Context, dest any, path string, query url.Values, headers map[string]string) error
	Post(ctx context.Context, dest any, path string, body any, headers map[string]string) error
	Put(ctx context.Context, dest any, path string, body any, headers map[string]string) error
	Patch(ctx context.Context, dest any, path string, body any, headers map[string]string) error
	Delete(ctx context.Context, dest any, path string, headers map[string]string) error
}

// StatusError is returned when the remote side answers with a non-2xx status.
type StatusError struct {
	Method     string
	URL        string
	StatusCode int
	Body       []byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("remote: %s %s: status %d: %s", e.Method, e.URL, e.StatusCode, strings.TrimSpace(string(e.Body)))
}

// Option configures an HTTPClient.
type Option func(*HTTPClient)

// WithTransport sets the RoundTripper chain (logging, metrics, ...).
func WithTransport(rt http.RoundTripper) Option {
	return func(c *HTTPClient) { c.http.Transport = rt }
}

// WithTimeout overrides the whole-request timeout (default 15s).
func WithTimeout(d time.Duration) Option {
	return func(c *HTTPClient) { c.http.Timeout = d }
}

// WithHeader adds a header sent with every request (e.g. an API key).
func WithHeader(key, value string) Option {
	return func(c *HTTPClient) { c.headers[key] = value }
}

// HTTPClient implements Client.
type HTTPClient struct {
	baseURL string
	http    *http.Client
	headers map[string]string
}

// New creates a client for baseURL (e.g. "https://api.example.com/v1").
func New(baseURL string, opts ...Option) *HTTPClient {
	c := &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: defaultTimeout},
		headers: map[string]string{},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *HTTPClient) Get(ctx context.Context, dest any, path string, query url.Values, headers map[string]string) error {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	return c.do(ctx, http.MethodGet, dest, path, nil, headers)
}

func (c *HTTPClient) Post(ctx context.Context, dest any, path string, body any, headers map[string]string) error {
	return c.do(ctx, http.MethodPost, dest, path, body, headers)
}

func (c *HTTPClient) Put(ctx context.Context, dest any, path string, body any, headers map[string]string) error {
	return c.do(ctx, http.MethodPut, dest, path, body, headers)
}

func (c *HTTPClient) Patch(ctx context.Context, dest any, path string, body any, headers map[string]string) error {
	return c.do(ctx, http.MethodPatch, dest, path, body, headers)
}

func (c *HTTPClient) Delete(ctx context.Context, dest any, path string, headers map[string]string) error {
	return c.do(ctx, http.MethodDelete, dest, path, nil, headers)
}

func (c *HTTPClient) do(ctx context.Context, method string, dest any, path string, body any, headers map[string]string) error {
	var reader io.Reader

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("remote: encode request body: %w", err)
		}

		reader = bytes.NewReader(raw)
	}

	fullURL := c.baseURL + "/" + strings.TrimLeft(path, "/")

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return fmt.Errorf("remote: build request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("remote: %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))

		return &StatusError{Method: method, URL: fullURL, StatusCode: resp.StatusCode, Body: raw}
	}

	if dest == nil {
		_, _ = io.Copy(io.Discard, resp.Body)

		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("remote: decode response: %w", err)
	}

	return nil
}
