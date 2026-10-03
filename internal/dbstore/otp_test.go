package dbstore

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
)

func TestOTPRepo_SendStats(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		const phone, ip = "+998900000101", "203.0.113.9"

		since := time.Now().Add(-time.Hour)

		st, err := s.OTP().SendStats(ctx, phone, domain.OTPPurposeSignUp, ip, since)
		if err != nil || st.LastSentAt != nil || st.PhoneCount != 0 || st.IPCount != 0 {
			t.Fatalf("empty stats: %+v, %v", st, err)
		}

		if err := s.OTP().LockPhone(ctx, phone); err != nil {
			t.Fatalf("LockPhone: %v", err)
		}

		create := func(purpose domain.OTPPurpose, ip string) domain.OTP {
			id := uuid.NewString()

			o, err := s.OTP().Create(ctx, domain.OTP{
				ID: id, PhoneNumber: phone, Purpose: purpose, CodeHash: domain.HashOTPCode(id, "123456"),
				IPAddress: ip, ExpiresAt: time.Now().Add(3 * time.Minute), CreatedAt: time.Now(),
			})
			if err != nil {
				t.Fatalf("Create: %+v, %v", o, err)
			}

			return o
		}

		create(domain.OTPPurposeSignUp, ip)
		create(domain.OTPPurposeUpdateUser, "2001:db8::1")
		deleted := create(domain.OTPPurposeSignUp, ip)

		if err := s.OTP().Delete(ctx, deleted.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		st, err = s.OTP().SendStats(ctx, phone, domain.OTPPurposeSignUp, ip, since)
		if err != nil || st.LastSentAt == nil || st.PhoneCount != 2 || st.IPCount != 1 {
			t.Fatalf("stats: %+v, %v", st, err)
		}
	})
}

func TestUserRepo_ExistsByPhone(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		const phone = "+998900000102"

		if ok, err := s.User().ExistsByPhone(ctx, phone); err != nil || ok {
			t.Fatalf("before create: %v, %v", ok, err)
		}

		u, err := s.User().Create(ctx, domain.User{PhoneNumber: strPtr(phone)})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if ok, err := s.User().ExistsByPhone(ctx, phone); err != nil || !ok {
			t.Fatalf("after create: %v, %v", ok, err)
		}

		if err := s.User().Delete(ctx, u.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if ok, err := s.User().ExistsByPhone(ctx, phone); err != nil || ok {
			t.Fatalf("after delete: %v, %v", ok, err)
		}
	})
}
