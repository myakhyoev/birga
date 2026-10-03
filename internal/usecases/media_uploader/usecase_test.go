package mediauploader

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// pngHeader is enough for content sniffing to report image/png.
var pngHeader = []byte("\x89PNG\x0D\x0A\x1A\x0A\x00\x00\x00\x0DIHDR")

type mockRepo struct {
	got domain.Media
	err error
}

func (m *mockRepo) Create(_ context.Context, md domain.Media) (domain.Media, error) {
	m.got = md

	return md, m.err
}

type mockStorage struct {
	putKey, putType, deleted string
	putData                  []byte
	err                      error
}

func (m *mockStorage) Put(_ context.Context, key, contentType string, data []byte) error {
	m.putKey, m.putType, m.putData = key, contentType, data

	return m.err
}

func (m *mockStorage) Delete(_ context.Context, key string) error {
	m.deleted = key

	return nil
}

func (m *mockStorage) URL(key string) string { return "https://cdn.test/" + key }

func TestExecute_Uploads(t *testing.T) {
	repo, st := &mockRepo{}, &mockStorage{}

	m, err := New(zap.NewNop(), repo, st, 1024, "dev/").Execute(context.Background(), domain.MediaUpload{Data: pngHeader})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if st.putType != "image/png" || !bytes.Equal(st.putData, pngHeader) {
		t.Fatalf("put %q %q", st.putType, st.putData)
	}

	if !strings.HasPrefix(st.putKey, "dev/media/") || !strings.HasSuffix(st.putKey, ".png") || !strings.Contains(st.putKey, m.ID) {
		t.Fatalf("key = %q, id = %q", st.putKey, m.ID)
	}

	if repo.got.Key != st.putKey || repo.got.Size != int64(len(pngHeader)) || repo.got.ContentType != "image/png" {
		t.Fatalf("stored %+v", repo.got)
	}

	if m.URL != "https://cdn.test/"+st.putKey {
		t.Fatalf("url = %q", m.URL)
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string][]byte{
		"empty":     nil,
		"too large": append(append([]byte{}, pngHeader...), make([]byte, 1024)...),
		"text":      []byte("hello, this is not an image"),
		"gif":       []byte("GIF89a......"),
	}

	for name, data := range cases {
		st := &mockStorage{}

		_, err := New(zap.NewNop(), &mockRepo{}, st, 1024, "").Execute(context.Background(), domain.MediaUpload{Data: data})
		if !errors.Is(err, errs.ErrValidation) {
			t.Errorf("%s: expected validation error, got %v", name, err)
		}

		if st.putKey != "" {
			t.Errorf("%s: uploaded although invalid", name)
		}
	}
}

func TestExecute_StorageFails(t *testing.T) {
	repo, st := &mockRepo{}, &mockStorage{err: errs.Errf(errs.ErrConnection, "down")}

	_, err := New(zap.NewNop(), repo, st, 1024, "").Execute(context.Background(), domain.MediaUpload{Data: pngHeader})
	if !errors.Is(err, errs.ErrConnection) || repo.got.Key != "" {
		t.Fatalf("err = %v, stored %+v", err, repo.got)
	}
}

func TestExecute_RepoFailsDeletesObject(t *testing.T) {
	repo, st := &mockRepo{err: errs.Errf(errs.ErrInternal, "db down")}, &mockStorage{}

	_, err := New(zap.NewNop(), repo, st, 1024, "").Execute(context.Background(), domain.MediaUpload{Data: pngHeader})
	if !errors.Is(err, errs.ErrInternal) || st.deleted != st.putKey {
		t.Fatalf("err = %v, deleted %q, put %q", err, st.deleted, st.putKey)
	}
}
