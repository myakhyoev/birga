package mediavariants

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
)

type fakeRepo struct {
	pending  []domain.Media
	saved    map[string]map[string]string
	failed   map[string]bool // id -> final
	claimErr error
}

func (r *fakeRepo) ClaimPending(_ context.Context, _ int, _ time.Duration) (domain.Media, bool, error) {
	if r.claimErr != nil {
		return domain.Media{}, false, r.claimErr
	}

	if len(r.pending) == 0 {
		return domain.Media{}, false, nil
	}

	m := r.pending[0]
	r.pending = r.pending[1:]

	return m, true, nil
}

func (r *fakeRepo) SaveVariants(_ context.Context, id string, v map[string]string) error {
	r.saved[id] = v

	return nil
}

func (r *fakeRepo) FailVariants(_ context.Context, id, _ string, final bool, _ int) error {
	r.failed[id] = final

	return nil
}

type fakeStorage struct {
	objects map[string][]byte
	getErr  error
}

func (s *fakeStorage) Get(_ context.Context, key string) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}

	return s.objects[key], nil
}

func (s *fakeStorage) Put(_ context.Context, key, _ string, data []byte) error {
	s.objects[key] = data

	return nil
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func size(t *testing.T, data []byte) (int, int) {
	t.Helper()

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	return cfg.Width, cfg.Height
}

func TestProcessOne_MakesTheSizesOfEachKind(t *testing.T) {
	st := &fakeStorage{objects: map[string][]byte{
		"dev/media/c.png": pngOf(t, 1536, 1024),
		"dev/media/p.png": pngOf(t, 600, 900),
	}}
	repo := &fakeRepo{saved: map[string]map[string]string{}, failed: map[string]bool{}, pending: []domain.Media{
		{ID: "c", Key: "dev/media/c.png", Kind: domain.MediaKindContent, Crop: &domain.MediaCrop{X: 0, Y: 0, W: 1, H: 0.75}},
		{ID: "p", Key: "dev/media/p.png", Kind: domain.MediaKindProfile},
	}}
	uc := New(zap.NewNop(), repo, st, "other/")

	for range 2 {
		if did, err := uc.ProcessOne(context.Background()); !did || err != nil {
			t.Fatalf("ProcessOne: %v %v", did, err)
		}
	}

	if did, _ := uc.ProcessOne(context.Background()); did {
		t.Fatal("nothing should be left")
	}

	want := map[string]map[string][2]int{
		"c": {"main": {1200, 600}, "thumb": {400, 200}, "small": {240, 240}},
		"p": {"main": {600, 600}, "thumb": {250, 250}}, // 600 wide source: main is not blown up
	}

	for id, sizes := range want {
		if len(repo.saved[id]) != len(sizes) {
			t.Fatalf("%s saved %v", id, repo.saved[id])
		}

		for name, wh := range sizes {
			key := repo.saved[id][name]
			if key != "dev/media/"+id+"/"+name+".jpg" { // the original's prefix, not the configured one
				t.Errorf("%s %s key %q", id, name, key)
			}

			if w, h := size(t, st.objects[key]); w != wh[0] || h != wh[1] {
				t.Errorf("%s %s is %dx%d, want %v", id, name, w, h, wh)
			}
		}
	}
}

func TestProcessOne_Failures(t *testing.T) {
	repo := &fakeRepo{saved: map[string]map[string]string{}, failed: map[string]bool{}, pending: []domain.Media{
		{ID: "bad", Key: "media/bad.png", Kind: domain.MediaKindContent},
		{ID: "net", Key: "media/net.png", Kind: domain.MediaKindContent},
	}}
	st := &fakeStorage{objects: map[string][]byte{"media/bad.png": []byte("not an image")}}
	uc := New(zap.NewNop(), repo, st, "")

	_, _ = uc.ProcessOne(context.Background())

	st.getErr = errors.New("timeout")
	_, _ = uc.ProcessOne(context.Background())

	if final, ok := repo.failed["bad"]; !ok || !final {
		t.Errorf("an unreadable image fails for good: %v %v", ok, final)
	}

	if final, ok := repo.failed["net"]; !ok || final {
		t.Errorf("a storage error is retried: %v %v", ok, final)
	}
}

func TestRun_StopsWithContext(t *testing.T) {
	uc := New(zap.NewNop(), &fakeRepo{saved: map[string]map[string]string{}, failed: map[string]bool{}}, &fakeStorage{}, "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		uc.Run(ctx, time.Hour)
		close(done)
	}()

	uc.Wake()
	uc.Wake() // never blocks
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}
