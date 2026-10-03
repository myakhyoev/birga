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
	got []byte
	err error
}

func (f *fakeMedia) Execute(_ context.Context, up domain.MediaUpload) (domain.Media, error) {
	f.got = up.Data

	return domain.Media{ID: testID, URL: "https://cdn.test/media/" + testID + ".png", ContentType: "image/png", Size: int64(len(up.Data))}, f.err
}

func (f *fakeMedia) MaxSize() int64 { return testMaxSize }

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
	s := New(config.Application{}, nil, &fakeHealth{}, &fakeCreator{}, &fakeLister{}, &fakeGetter{}, UserUseCases{}, OTPUseCases{}, nil)

	ct, b := multipartBody(t, []byte("x"), "")
	if code, r := doRaw(t, s, ct, b); code != http.StatusServiceUnavailable || r.ErrorCode != _errCodeUnavailable {
		t.Fatalf("disabled: %d %+v", code, r)
	}
}
