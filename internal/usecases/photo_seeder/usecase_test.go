package photoseeder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
)

type fakeRepo struct {
	need map[string]bool
	set  map[string]string
}

func (f *fakeRepo) NeedsPhoto(_ context.Context, kind, key string) (bool, error) {
	return f.need[kind+"/"+key], nil
}

func (f *fakeRepo) SetPhoto(_ context.Context, kind, key, mediaID string) (int, error) {
	f.set[kind+"/"+key] = mediaID

	return 1, nil
}

type fakeUploader struct{ n int }

func (f *fakeUploader) Execute(_ context.Context, up domain.MediaUpload) (domain.Media, error) {
	f.n++

	return domain.Media{ID: "m-" + string(up.Data)}, nil
}

func TestExecute(t *testing.T) {
	dir := t.TempDir()
	list := "# comment\n" +
		"activity\tQutidan uy\tactivities/qutidan-uy.jpg\thttps://example.com/1.png\n" +
		"article\t5a2e0000-0000-4000-8000-000000000001\tarticles/01.jpg\thttps://example.com/2.png\n" +
		"post\t5a3e0000-0000-4000-8000-000000000001\tposts/01.jpg\thttps://example.com/3.png\n\n"

	if err := os.WriteFile(filepath.Join(dir, SourcesFile), []byte(list), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "activities"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "activities", "qutidan-uy.jpg"), []byte("uy"), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := &fakeRepo{
		need: map[string]bool{
			"activity/Qutidan uy":                          true,
			"post/5a3e0000-0000-4000-8000-000000000001":    true, // file not fetched yet
			"article/5a2e0000-0000-4000-8000-000000000001": false,
		},
		set: map[string]string{},
	}
	up := &fakeUploader{}

	res, err := New(zap.NewNop(), repo, up).Execute(context.Background(), dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if res != (Result{Listed: 3, Attached: 1, Missing: 1}) {
		t.Fatalf("result: %+v", res)
	}

	if up.n != 1 || repo.set["activity/Qutidan uy"] != "m-uy" || len(repo.set) != 1 {
		t.Fatalf("uploads %d, set %v", up.n, repo.set)
	}
}

func TestParseSourcesRejectsBadLines(t *testing.T) {
	for name, in := range map[string]string{
		"too few columns": "activity\tQutidan uy\n",
		"unknown kind":    "tip\tx\ttips/x.jpg\n",
	} {
		if _, err := ParseSources(strings.NewReader(in)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestCommittedSourcesParse(t *testing.T) {
	f, err := os.Open("../../../seeds/photos/" + SourcesFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	photos, err := ParseSources(f)
	if err != nil || len(photos) == 0 {
		t.Fatalf("seeds/photos/%s: %d photos, %v", SourcesFile, len(photos), err)
	}
}
