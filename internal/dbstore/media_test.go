package dbstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func TestMediaRepo_CreateAndUserPhoto(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		id := "5b3c8a0e-1f2d-4c6b-9e7a-2d4f6a8c0b1e"

		m, err := s.Media().Create(ctx, domain.Media{ID: id, Key: "media/" + id + ".png", ContentType: "image/png", Size: 42})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if m.ID != id || m.Key != "media/"+id+".png" || m.Size != 42 || m.CreatedAt.IsZero() {
			t.Fatalf("unexpected media: %+v", m)
		}

		if got, err := s.Media().Get(ctx, id); err != nil || got.Key != m.Key {
			t.Fatalf("Get: %+v, %v", got, err)
		}

		if _, err := s.Media().Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, errs.ErrMediaNotFound) {
			t.Fatalf("Get unknown: %v", err)
		}

		u, err := s.User().Create(ctx, domain.User{PhoneNumber: strPtr("+998900000009"), PhotoID: &id})
		if err != nil || *u.PhotoID != id {
			t.Fatalf("user with photo: %+v, %v", u, err)
		}

		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "SAVEPOINT sp"); err != nil {
			t.Fatalf("savepoint: %v", err)
		}

		unknown := "00000000-0000-0000-0000-000000000000"
		if _, err := s.User().Update(ctx, u.ID, domain.UserUpdate{PhotoID: &unknown}); !errors.Is(err, errs.ErrPhotoNotFound) {
			t.Fatalf("expected ErrPhotoNotFound, got %v", err)
		}

		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, "ROLLBACK TO SAVEPOINT sp"); err != nil {
			t.Fatalf("rollback to savepoint: %v", err)
		}
	})
}

func TestMediaRepo_Variants(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		const id = "6c4d9b1f-2a3e-4d7c-8f8b-3e5a7b9d1c2f"

		crop := &domain.MediaCrop{X: 0, Y: 0.1, W: 1, H: 0.5}

		m, err := s.Media().Create(ctx, domain.Media{ID: id, Key: "media/" + id + ".png", ContentType: "image/png", Size: 42,
			Kind: domain.MediaKindContent, Crop: crop})
		if err != nil || m.Kind != domain.MediaKindContent || m.Crop == nil || *m.Crop != *crop || m.VariantsStatus != domain.VariantsPending {
			t.Fatalf("Create: %+v, %v", m, err)
		}

		video, err := s.Media().Create(ctx, domain.Media{Key: "media/v.mp4", ContentType: "video/mp4", Size: 42})
		if err != nil || video.VariantsStatus != domain.VariantsNone || video.Kind != "" || video.Crop != nil {
			t.Fatalf("video: %+v, %v", video, err)
		}

		got, ok, err := s.Media().ClaimPending(ctx, 5, time.Minute)
		if err != nil || !ok || got.ID != id {
			t.Fatalf("ClaimPending: %+v %v %v", got, ok, err)
		}

		// Claimed a moment ago: not handed out again until the claim is stale.
		if _, ok, err := s.Media().ClaimPending(ctx, 5, time.Minute); ok || err != nil {
			t.Fatalf("claimed twice: %v %v", ok, err)
		}

		// A retryable failure keeps it pending; a stale claim is taken again.
		if err := s.Media().FailVariants(ctx, id, "timeout", false, 5); err != nil {
			t.Fatal(err)
		}

		if again, ok, err := s.Media().ClaimPending(ctx, 5, 0); !ok || err != nil || again.ID != id {
			t.Fatalf("stale claim: %v %v", ok, err)
		}

		v := map[string]string{"main": "media/" + id + "/main.jpg", "thumb": "media/" + id + "/thumb.jpg"}
		if err := s.Media().SaveVariants(ctx, id, v); err != nil {
			t.Fatal(err)
		}

		done, err := s.Media().Get(ctx, id)
		if err != nil || done.VariantsStatus != domain.VariantsDone || done.Variants["thumb"] != v["thumb"] {
			t.Fatalf("after save: %+v %v", done, err)
		}

		if _, ok, _ := s.Media().ClaimPending(ctx, 5, 0); ok {
			t.Fatal("done rows are not pending")
		}

		// Out of attempts: failed for good.
		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `UPDATE media SET variants_status = 'pending', variants_attempts = 5 WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}

		if err := s.Media().FailVariants(ctx, id, "timeout", false, 5); err != nil {
			t.Fatal(err)
		}

		if f, _ := s.Media().Get(ctx, id); f.VariantsStatus != domain.VariantsFailed {
			t.Fatalf("status %q", f.VariantsStatus)
		}
	})
}
