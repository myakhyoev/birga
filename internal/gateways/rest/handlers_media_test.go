package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const testMaxSize = 1024

type fakeMedia struct {
	got        []byte
	allowVideo bool
	kind       domain.MediaKind
	crop       *domain.MediaCrop
	recropID   string
	err        error
}

func (f *fakeMedia) Execute(_ context.Context, up domain.MediaUpload) (domain.Media, error) {
	f.got, f.allowVideo, f.kind, f.crop = up.Data, up.AllowVideo, up.Kind, up.Crop

	return domain.Media{ID: testID, URL: "https://cdn.test/media/" + testID + ".png", ContentType: "image/png", Size: int64(len(up.Data))}, f.err
}

func (f *fakeMedia) MaxSize() int64 { return testMaxSize }

func (f *fakeMedia) Recrop(_ context.Context, id string, kind domain.MediaKind, crop *domain.MediaCrop) (domain.Media, error) {
	f.recropID, f.kind, f.crop = id, kind, crop

	return domain.Media{ID: testID, ContentType: "image/jpeg", Size: 10, Kind: kind, VariantsStatus: domain.VariantsPending}, f.err
}

// Link serves the thumb as made, and every other size from the original.
func (f *fakeMedia) Link(_ context.Context, id, size string) (string, string, error) {
	if id != testID {
		return "", "", errs.ErrMediaNotFound
	}

	if size == domain.MediaSizeThumb {
		return "https://signed.test/" + id + "/thumb.jpg", size, nil
	}

	return "https://signed.test/" + id, domain.MediaSizeOriginal, nil
}

func TestGetMedia_Size(t *testing.T) {
	s, _ := newTestServer("")

	for _, tt := range []struct{ size, location, served string }{
		{"thumb", "https://signed.test/" + testID + "/thumb.jpg", "thumb"},
		{"main", "https://signed.test/" + testID, "original"}, // not made yet: the original
	} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/media/"+testID+"?size="+tt.size, nil))

		if w.Code != http.StatusFound || w.Header().Get("Location") != tt.location || w.Header().Get("X-Media-Variant") != tt.served {
			t.Fatalf("size %s: code %d location %q served %q", tt.size, w.Code, w.Header().Get("Location"), w.Header().Get("X-Media-Variant"))
		}
	}

	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/media/"+testID+"?size=huge", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad size: %d", w.Code)
	}
}

func TestGetMedia(t *testing.T) {
	s, _ := newTestServer("")

	req := httptest.NewRequest(http.MethodGet, "/v1/media/"+testID, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://signed.test/"+testID {
		t.Fatalf("code %d location %q", w.Code, w.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/media/other", nil)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", w.Code)
	}
}

func doRaw(t *testing.T, s http.Handler, contentType string, body []byte) (int, R) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/media", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var r R
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("response is not JSON envelope: %q", w.Body.String())
	}

	return w.Code, r
}

func multipartBody(t *testing.T, file []byte, base64Value string) (string, []byte) {
	t.Helper()

	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	if file != nil {
		fw, err := mw.CreateFormFile("file", "photo.png")
		if err != nil {
			t.Fatal(err)
		}

		_, _ = fw.Write(file)
	} else {
		_ = mw.WriteField("file", base64Value)
	}

	_ = mw.Close()

	return mw.FormDataContentType(), buf.Bytes()
}

func TestUploadMedia(t *testing.T) {
	photo := []byte("\x89PNG\x0D\x0A\x1A\x0A photo bytes")
	b64 := base64.StdEncoding.EncodeToString(photo)

	form := func(v string) (string, []byte) {
		return "application/x-www-form-urlencoded", []byte(url.Values{"file": {v}}.Encode())
	}

	cases := map[string]func() (string, []byte){
		"multipart file":         func() (string, []byte) { return multipartBody(t, photo, "") },
		"multipart base64":       func() (string, []byte) { return multipartBody(t, nil, b64) },
		"urlencoded base64":      func() (string, []byte) { return form(b64) },
		"urlencoded data url":    func() (string, []byte) { return form("data:image/png;base64," + b64) },
		"urlencoded raw url b64": func() (string, []byte) { return form(base64.RawURLEncoding.EncodeToString(photo)) },
	}

	for name, body := range cases {
		s, d := newTestServer("")

		ct, b := body()

		code, r := doRaw(t, s, ct, b)
		if code != http.StatusOK || !bytes.Equal(d.media.got, photo) {
			t.Errorf("%s: %d %+v, got %q", name, code, r, d.media.got)

			continue
		}

		data, _ := r.Data.(map[string]any)
		if data["id"] != testID || data["url"] == "" || data["content_type"] != "image/png" {
			t.Errorf("%s: unexpected data %+v", name, r.Data)
		}
	}
}

func TestUploadMedia_Errors(t *testing.T) {
	s, d := newTestServer("")

	if code, r := doRaw(t, s, "application/x-www-form-urlencoded", []byte("other=1")); code != http.StatusBadRequest {
		t.Errorf("missing field: %d %+v", code, r)
	}

	if code, r := doRaw(t, s, "application/x-www-form-urlencoded", []byte("file=%%%not-base64")); code != http.StatusBadRequest {
		t.Errorf("bad base64: %d %+v", code, r)
	}

	big := base64.StdEncoding.EncodeToString(make([]byte, 200<<10))
	if code, r := doRaw(t, s, "application/x-www-form-urlencoded", []byte(url.Values{"file": {big}}.Encode())); code != http.StatusUnprocessableEntity {
		t.Errorf("too large: %d %+v", code, r)
	}

	ct, b := multipartBody(t, []byte("not an image"), "")
	d.media.err = errs.ErrMediaUnsupportedType

	if code, r := doRaw(t, s, ct, b); code != http.StatusUnprocessableEntity || r.ErrorNote != errs.ErrMediaUnsupportedType.Error() {
		t.Errorf("unsupported: %d %+v", code, r)
	}
}

func TestUploadMedia_Disabled(t *testing.T) {
	s := New(config.Application{}, nil, &fakeHealth{}, &fakeCreator{}, &fakeLister{}, &fakeGetter{}, ActivityEditUseCases{}, GoalUseCases{}, ChildActivityUseCases{}, UserUseCases{}, MeUseCases{}, OTPUseCases{}, AuthUseCases{}, ArticleUseCases{}, PostUseCases{}, TipUseCases{}, nil, nil)

	ct, b := multipartBody(t, []byte("x"), "")
	if code, r := doRaw(t, s, ct, b); code != http.StatusServiceUnavailable || r.ErrorCode != _errCodeUnavailable {
		t.Fatalf("disabled: %d %+v", code, r)
	}
}

func TestAdminUploadMedia_AllowsVideo(t *testing.T) {
	s, d := newTestServer(testAdminKey)
	ct, b := multipartBody(t, []byte("video"), "")

	upload := func(headers map[string]string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/media", bytes.NewReader(b))
		req.Header.Set("Content-Type", ct)

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		return w.Code
	}

	if code := upload(nil); code != http.StatusUnauthorized {
		t.Fatalf("without admin auth: %d", code)
	}

	if code := upload(map[string]string{adminKeyHeader: testAdminKey}); code != http.StatusOK || !d.media.allowVideo {
		t.Fatalf("admin upload: %d allowVideo=%v", code, d.media.allowVideo)
	}

	if code, _ := doRaw(t, s, ct, b); code != http.StatusOK || d.media.allowVideo {
		t.Fatalf("public upload must stay images only: %d allowVideo=%v", code, d.media.allowVideo)
	}
}

func TestUploadMedia_KindAndCrop(t *testing.T) {
	photo := []byte("\x89PNG\x0D\x0A\x1A\x0A photo bytes")
	b64 := base64.StdEncoding.EncodeToString(photo)

	upload := func(s http.Handler, path string, v url.Values) (int, R) {
		v.Set("file", b64)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(v.Encode())))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(adminKeyHeader, testAdminKey)

		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		var r R
		_ = json.Unmarshal(w.Body.Bytes(), &r)

		return w.Code, r
	}

	s, d := newTestServer(testAdminKey)

	// Defaults: the app uploads profile photos, the dashboard content pictures.
	if code, r := upload(s, "/v1/media", url.Values{}); code != http.StatusOK || d.media.kind != domain.MediaKindProfile || d.media.crop != nil {
		t.Fatalf("app default: %d %+v kind %q", code, r, d.media.kind)
	}

	if code, r := upload(s, "/v1/admin/media", url.Values{}); code != http.StatusOK || d.media.kind != domain.MediaKindContent {
		t.Fatalf("admin default: %d %+v kind %q", code, r, d.media.kind)
	}

	code, r := upload(s, "/v1/admin/media", url.Values{"kind": {"profile"}, "crop": {"0.1, 0.2, 0.5, 0.5"}})
	if code != http.StatusOK || d.media.kind != domain.MediaKindProfile || d.media.crop == nil || *d.media.crop != (domain.MediaCrop{X: 0.1, Y: 0.2, W: 0.5, H: 0.5}) {
		t.Fatalf("kind and crop: %d %+v kind %q crop %+v", code, r, d.media.kind, d.media.crop)
	}

	for _, bad := range []url.Values{
		{"kind": {"banner"}},
		{"crop": {"0,0,1"}},
		{"crop": {"0.5,0,0.8,1"}},
		{"crop": {"a,b,c,d"}},
	} {
		if code, r := upload(s, "/v1/admin/media", bad); code != http.StatusUnprocessableEntity {
			t.Errorf("%v: %d %+v", bad, code, r)
		}
	}
}

func TestAdminRecropMedia(t *testing.T) {
	s, d := newTestServer(testAdminKey)

	recrop := func(body string) (int, R) {
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/media/"+testID+"/recrop", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(adminKeyHeader, testAdminKey)

		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		var r R
		_ = json.Unmarshal(w.Body.Bytes(), &r)

		return w.Code, r
	}

	code, r := recrop(`{"kind":"content","crop":{"x":0,"y":0.1,"w":1,"h":0.5}}`)
	if code != http.StatusOK || d.media.recropID != testID || d.media.crop == nil || d.media.crop.H != 0.5 {
		t.Fatalf("recrop: %d %+v %+v", code, r, d.media.crop)
	}

	data, _ := r.Data.(map[string]any)
	if data["variants_status"] != "pending" || data["kind"] != "content" {
		t.Fatalf("view: %+v", data)
	}

	if code, _ := recrop(`{"kind":"square"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad kind: %d", code)
	}
}
