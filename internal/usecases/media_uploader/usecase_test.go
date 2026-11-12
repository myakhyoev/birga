package mediauploader

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func (m *mockRepo) Get(_ context.Context, id string) (domain.Media, error) {
	if m.err != nil {
		return domain.Media{}, m.err
	}

	return domain.Media{ID: id, Key: "media/" + id + ".png"}, nil
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

func (m *mockStorage) Get(_ context.Context, key string) ([]byte, error) {
	return pngHeader, m.err
}

func (m *mockStorage) URL(key string) string { return "https://cdn.test/" + key }

func (m *mockStorage) Presign(_ context.Context, key string, ttl time.Duration) (string, error) {
	return "https://signed.test/" + key + "?ttl=" + ttl.String(), m.err
}

func TestExecute_Uploads(t *testing.T) {
	repo, st := &mockRepo{}, &mockStorage{}

	m, err := New(zap.NewNop(), repo, st, 1024, "dev/", time.Hour).Execute(context.Background(), domain.MediaUpload{Data: pngHeader})
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

		_, err := New(zap.NewNop(), &mockRepo{}, st, 1024, "", time.Hour).Execute(context.Background(), domain.MediaUpload{Data: data})
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

	_, err := New(zap.NewNop(), repo, st, 1024, "", time.Hour).Execute(context.Background(), domain.MediaUpload{Data: pngHeader})
	if !errors.Is(err, errs.ErrConnection) || repo.got.Key != "" {
		t.Fatalf("err = %v, stored %+v", err, repo.got)
	}
}

func TestExecute_RepoFailsDeletesObject(t *testing.T) {
	repo, st := &mockRepo{err: errs.Errf(errs.ErrInternal, "db down")}, &mockStorage{}

	_, err := New(zap.NewNop(), repo, st, 1024, "", time.Hour).Execute(context.Background(), domain.MediaUpload{Data: pngHeader})
	if !errors.Is(err, errs.ErrInternal) || st.deleted != st.putKey {
		t.Fatalf("err = %v, deleted %q, put %q", err, st.deleted, st.putKey)
	}
}

func TestLink(t *testing.T) {
	uc := New(zap.NewNop(), &mockRepo{}, &mockStorage{}, 1024, "", time.Hour)

	const id = "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"

	link, served, err := uc.Link(context.Background(), id, "thumb")
	if err != nil || link != "https://signed.test/media/"+id+".png?ttl=1h0m0s" || served != "original" {
		t.Fatalf("link %q %q, %v", link, served, err)
	}

	if _, _, err := uc.Link(context.Background(), "not-a-uuid", ""); !errors.Is(err, errs.ErrMediaNotFound) {
		t.Fatalf("bad id: %v", err)
	}
}

// mp4Header is the start of an MP4 file: an ftyp box with the "mp42" brand.
var mp4Header = []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")

func TestExecute_Video(t *testing.T) {
	st := &mockStorage{}

	if _, err := New(zap.NewNop(), &mockRepo{}, st, 1024, "", time.Hour).Execute(context.Background(),
		domain.MediaUpload{Data: mp4Header}); !errors.Is(err, errs.ErrMediaUnsupportedType) || st.putKey != "" {
		t.Fatalf("a video without AllowVideo must be refused: %v", err)
	}

	if _, err := New(zap.NewNop(), &mockRepo{}, st, 1024, "", time.Hour).Execute(context.Background(),
		domain.MediaUpload{Data: mp4Header, AllowVideo: true}); err != nil || st.putType != "video/mp4" || !strings.HasSuffix(st.putKey, ".mp4") {
		t.Fatalf("video upload: %v, type %q, key %q", err, st.putType, st.putKey)
	}

	if _, err := New(zap.NewNop(), &mockRepo{}, &mockStorage{}, 1024, "", time.Hour).Execute(context.Background(),
		domain.MediaUpload{Data: []byte("GIF89a......"), AllowVideo: true}); !errors.Is(err, errs.ErrMediaUnsupportedTypeOrVideo) {
		t.Fatalf("gif with AllowVideo: %v", err)
	}
}

func TestLink_Variant(t *testing.T) {
	const id = "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"

	repo := &variantRepo{m: domain.Media{ID: id, Key: "media/" + id + ".png", Kind: domain.MediaKindProfile,
		Variants: map[string]string{"main": "media/" + id + "/main.jpg", "thumb": "media/" + id + "/thumb.jpg"}}}
	uc := New(zap.NewNop(), repo, &mockStorage{}, 1024, "", time.Hour)

	for size, want := range map[string]string{"main": "main", "thumb": "thumb", "small": "thumb", "original": "original", "": "original"} {
		link, served, err := uc.Link(context.Background(), id, size)
		if err != nil || served != want || !strings.Contains(link, map[string]string{"main": "/main.jpg", "thumb": "/thumb.jpg", "original": ".png"}[want]) {
			t.Errorf("size %q: %q %q %v", size, link, served, err)
		}
	}
}

type variantRepo struct {
	m       domain.Media
	created domain.Media
}

func (r *variantRepo) Create(_ context.Context, m domain.Media) (domain.Media, error) {
	r.created = m

	return m, nil
}

func (r *variantRepo) Get(_ context.Context, _ string) (domain.Media, error) { return r.m, nil }

func TestExecute_KindCropAndWake(t *testing.T) {
	repo, st := &mockRepo{}, &mockStorage{}
	uc := New(zap.NewNop(), repo, st, 1024, "", time.Hour)

	woken := 0
	uc.OnUpload(func() { woken++ })

	crop := &domain.MediaCrop{X: 0, Y: 0.1, W: 1, H: 0.5}
	if _, err := uc.Execute(context.Background(), domain.MediaUpload{Data: pngHeader, Kind: domain.MediaKindContent, Crop: crop}); err != nil {
		t.Fatal(err)
	}

	if repo.got.Kind != domain.MediaKindContent || repo.got.Crop != crop || woken != 1 {
		t.Fatalf("stored %+v, woken %d", repo.got, woken)
	}

	for _, up := range []domain.MediaUpload{
		{Data: pngHeader, Crop: crop},     // crop without kind
		{Data: pngHeader, Kind: "banner"}, // unknown kind
		{Data: pngHeader, Kind: domain.MediaKindContent, Crop: &domain.MediaCrop{X: 0.5, W: 0.8, H: 1}}, // outside
	} {
		if _, err := uc.Execute(context.Background(), up); !errors.Is(err, errs.ErrValidation) {
			t.Errorf("%+v: %v", up, err)
		}
	}

	// Videos keep no kind and do not wake the worker.
	if _, err := New(zap.NewNop(), repo, st, 1024, "", time.Hour).Execute(context.Background(),
		domain.MediaUpload{Data: mp4Header, AllowVideo: true, Kind: domain.MediaKindContent}); err != nil || repo.got.Kind != "" {
		t.Fatalf("video: %v kind %q", err, repo.got.Kind)
	}
}

func TestRecrop(t *testing.T) {
	const id = "3f1d2c4b-8a9e-4b7c-9d2e-1a2b3c4d5e6f"

	repo := &variantRepo{m: domain.Media{ID: id, Key: "media/" + id + ".png", ContentType: "image/png"}}
	st := &mockStorage{}
	uc := New(zap.NewNop(), repo, st, 1024, "", time.Hour)

	crop := &domain.MediaCrop{X: 0, Y: 0, W: 1, H: 0.5}

	m, err := uc.Recrop(context.Background(), id, domain.MediaKindContent, crop)
	if err != nil || m.ID == id || repo.created.Kind != domain.MediaKindContent || repo.created.Crop != crop || !bytes.Equal(st.putData, pngHeader) {
		t.Fatalf("recrop: %+v %v created %+v", m, err, repo.created)
	}

	repo.m.ContentType = "video/mp4"
	if _, err := uc.Recrop(context.Background(), id, domain.MediaKindContent, nil); !errors.Is(err, errs.ErrValidation) {
		t.Fatalf("video: %v", err)
	}
}
