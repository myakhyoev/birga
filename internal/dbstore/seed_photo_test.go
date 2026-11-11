package dbstore

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

func TestActivityPhotoAndSeedPhotos(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		g, err := s.Goal().Create(ctx, domain.Goal{NameUz: "Rasm test", NameRu: "Фото тест", NameEn: "Photo test"})
		if err != nil {
			t.Fatalf("goal: %v", err)
		}

		photo := "6c4d9b1f-2e3a-4d7c-8f8b-3e5a7b9d1c2f"
		if _, err := s.Media().Create(ctx, domain.Media{ID: photo, Key: "media/" + photo + ".jpg", ContentType: "image/jpeg", Size: 1}); err != nil {
			t.Fatalf("media: %v", err)
		}

		a := activity(g.ID, 3, 5, true)
		a.TitleUz = "Yostiq yo‘lagi test"
		a.PhotoID = &photo

		created, err := s.Activity().Create(ctx, a)
		if err != nil || created.PhotoID == nil || *created.PhotoID != photo {
			t.Fatalf("create with photo: %+v, %v", created.PhotoID, err)
		}

		empty := ""

		cleared, err := s.Activity().Update(ctx, created.ID, domain.ActivityUpdate{PhotoID: &empty})
		if err != nil || cleared.PhotoID != nil {
			t.Fatalf("clear photo: %+v, %v", cleared.PhotoID, err)
		}

		missing := "00000000-0000-0000-0000-000000000000"

		// The seed list writes the straight apostrophe; the stored title uses ‘.
		need, err := s.SeedPhoto().NeedsPhoto(ctx, domain.SeedPhotoActivity, "Yostiq yo'lagi test")
		if err != nil || !need {
			t.Fatalf("NeedsPhoto: %v, %v", need, err)
		}

		n, err := s.SeedPhoto().SetPhoto(ctx, domain.SeedPhotoActivity, "Yostiq yo'lagi test", photo)
		if err != nil || n != 1 {
			t.Fatalf("SetPhoto: %d, %v", n, err)
		}

		got, err := s.Activity().Get(ctx, created.ID)
		if err != nil || got.PhotoID == nil || *got.PhotoID != photo {
			t.Fatalf("seeded photo: %+v, %v", got.PhotoID, err)
		}

		// A picture is never replaced.
		if need, _ := s.SeedPhoto().NeedsPhoto(ctx, domain.SeedPhotoActivity, "Yostiq yo'lagi test"); need {
			t.Fatal("NeedsPhoto after seeding: true")
		}

		if n, _ := s.SeedPhoto().SetPhoto(ctx, domain.SeedPhotoActivity, "Yostiq yo'lagi test", photo); n != 0 {
			t.Fatalf("SetPhoto twice changed %d rows", n)
		}

		if need, _ := s.SeedPhoto().NeedsPhoto(ctx, domain.SeedPhotoPost, missing); need {
			t.Fatal("NeedsPhoto for an unknown post: true")
		}

		if _, err := s.SeedPhoto().NeedsPhoto(ctx, "tip", "x"); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("unknown kind: %v", err)
		}

		// Last: the failed foreign key aborts the transaction.
		if _, err := s.Activity().Update(ctx, created.ID, domain.ActivityUpdate{PhotoID: &missing}); !errors.Is(err, errs.ErrPhotoNotFound) {
			t.Fatalf("unknown photo: %v", err)
		}
	})
}
