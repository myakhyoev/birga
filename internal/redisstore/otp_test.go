package redisstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const (
	phone = "+998901234567"
	ip    = "203.0.113.7"
)

var limits = domain.OTPLimits{ResendCooldown: time.Minute, Window: time.Hour, MaxPerPhone: 3, MaxPerIP: 4}

func newStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	return New(redis.NewClient(&redis.Options{Addr: mr.Addr()})), mr
}

func TestAcquireSend_Limits(t *testing.T) {
	s, mr := newStore(t)
	ctx := context.Background()

	if err := s.OTP().AcquireSend(ctx, phone, domain.OTPPurposeSignUp, ip, limits); err != nil {
		t.Fatalf("first send: %v", err)
	}

	err := s.OTP().AcquireSend(ctx, phone, domain.OTPPurposeSignUp, ip, limits)
	if !errors.Is(err, errs.ErrRateLimited) || err.Error() != "a code was sent recently, request a new one in 60 seconds" {
		t.Fatalf("cooldown: %v", err)
	}

	// The cooldown is per purpose; the phone counter is shared.
	if err := s.OTP().AcquireSend(ctx, phone, domain.OTPPurposeUpdateUser, ip, limits); err != nil {
		t.Fatalf("other purpose: %v", err)
	}

	mr.FastForward(time.Minute)

	if err := s.OTP().AcquireSend(ctx, phone, domain.OTPPurposeSignUp, ip, limits); err != nil {
		t.Fatalf("after cooldown: %v", err)
	}

	mr.FastForward(time.Minute)

	err = s.OTP().AcquireSend(ctx, phone, domain.OTPPurposeSignUp, ip, limits)
	if !errors.Is(err, errs.ErrRateLimited) || !strings.HasPrefix(err.Error(), "too many codes requested for this phone number, try again in 3480 seconds") {
		t.Fatalf("phone limit: %v", err)
	}

	// A fourth send from the same IP to another phone passes; the fifth hits the IP limit.
	if err := s.OTP().AcquireSend(ctx, "+998901111111", domain.OTPPurposeSignUp, ip, limits); err != nil {
		t.Fatalf("other phone: %v", err)
	}

	err = s.OTP().AcquireSend(ctx, "+998902222222", domain.OTPPurposeSignUp, ip, limits)
	if !errors.Is(err, errs.ErrRateLimited) || !strings.HasPrefix(err.Error(), "too many codes requested from this IP address") {
		t.Fatalf("ip limit: %v", err)
	}

	mr.FastForward(time.Hour)

	if err := s.OTP().AcquireSend(ctx, phone, domain.OTPPurposeSignUp, ip, limits); err != nil {
		t.Fatalf("after window: %v", err)
	}
}

func TestReleaseSend(t *testing.T) {
	s, mr := newStore(t)
	ctx := context.Background()

	if err := s.OTP().AcquireSend(ctx, phone, domain.OTPPurposeSignUp, ip, limits); err != nil {
		t.Fatalf("AcquireSend: %v", err)
	}

	if err := s.OTP().SaveCode(ctx, phone, domain.OTPPurposeSignUp, "h", time.Minute); err != nil {
		t.Fatalf("SaveCode: %v", err)
	}

	if err := s.OTP().ReleaseSend(ctx, phone, domain.OTPPurposeSignUp, ip); err != nil {
		t.Fatalf("ReleaseSend: %v", err)
	}

	if mr.Exists(codeKey(phone, domain.OTPPurposeSignUp)) || mr.Exists(cooldownKey(phone, domain.OTPPurposeSignUp)) {
		t.Fatalf("code or cooldown left behind: %v", mr.Keys())
	}

	if v, _ := mr.Get(phoneLimitKey(phone)); v != "0" {
		t.Fatalf("phone counter = %q, want 0", v)
	}

	// Releasing again never goes below zero.
	_ = s.OTP().ReleaseSend(ctx, phone, domain.OTPPurposeSignUp, ip)

	if v, _ := mr.Get(ipLimitKey(ip)); v != "0" {
		t.Fatalf("ip counter = %q, want 0", v)
	}
}

func TestVerifyCode(t *testing.T) {
	s, mr := newStore(t)
	ctx := context.Background()
	key := codeKey(phone, domain.OTPPurposeSignUp)

	verify := func(hash string) (domain.OTPVerifyOutcome, int) {
		t.Helper()

		out, left, err := s.OTP().VerifyCode(ctx, phone, domain.OTPPurposeSignUp, hash, 3, 10*time.Minute)
		if err != nil {
			t.Fatalf("VerifyCode: %v", err)
		}

		return out, left
	}

	if out, _ := verify("good"); out != domain.OTPNotFound {
		t.Fatalf("no code: %v", out)
	}

	if err := s.OTP().SaveCode(ctx, phone, domain.OTPPurposeSignUp, "good", 2*time.Minute); err != nil {
		t.Fatalf("SaveCode: %v", err)
	}

	if ttl := mr.TTL(key); ttl != 2*time.Minute {
		t.Fatalf("ttl = %v", ttl)
	}

	if out, left := verify("bad"); out != domain.OTPMismatch || left != 2 {
		t.Fatalf("first wrong: %v %d", out, left)
	}

	// Another purpose's code is not this one.
	if out, _, _ := s.OTP().VerifyCode(ctx, phone, domain.OTPPurposeUpdateUser, "good", 3, 10*time.Minute); out != domain.OTPNotFound {
		t.Fatalf("other purpose: %v", out)
	}

	if out, _ := verify("good"); out != domain.OTPMatched || mr.Exists(key) {
		t.Fatalf("match should delete the code: %v", out)
	}

	if out, _ := verify("good"); out != domain.OTPNotFound {
		t.Fatalf("second use: %v", out)
	}

	// Attempts reset with a new code; the last wrong attempt deletes it.
	_ = s.OTP().SaveCode(ctx, phone, domain.OTPPurposeSignUp, "good", 2*time.Minute)
	verify("bad")
	verify("bad")

	if out, _ := verify("bad"); out != domain.OTPTooManyTries || mr.Exists(key) {
		t.Fatalf("third wrong: %v", out)
	}

	// Expiry.
	_ = s.OTP().SaveCode(ctx, phone, domain.OTPPurposeSignUp, "good", 2*time.Minute)
	mr.FastForward(2 * time.Minute)

	if out, _ := verify("good"); out != domain.OTPNotFound {
		t.Fatalf("expired: %v", out)
	}
}

func TestVerifyCode_MarksVerified(t *testing.T) {
	s, mr := newStore(t)
	ctx := context.Background()

	isVerified := func(purpose domain.OTPPurpose) bool {
		t.Helper()

		v, err := s.OTP().IsVerified(ctx, phone, purpose)
		if err != nil {
			t.Fatalf("IsVerified: %v", err)
		}

		return v
	}

	_ = s.OTP().SaveCode(ctx, phone, domain.OTPPurposeSignUp, "good", 2*time.Minute)

	if out, _, _ := s.OTP().VerifyCode(ctx, phone, domain.OTPPurposeSignUp, "bad", 3, 10*time.Minute); out != domain.OTPMismatch || isVerified(domain.OTPPurposeSignUp) {
		t.Fatalf("wrong code must not verify: %v", out)
	}

	if out, _, _ := s.OTP().VerifyCode(ctx, phone, domain.OTPPurposeSignUp, "good", 3, 10*time.Minute); out != domain.OTPMatched {
		t.Fatalf("match: %v", out)
	}

	// A match marks the phone verified for this purpose only, for the verified TTL.
	if ttl := mr.TTL(verifiedKey(phone, domain.OTPPurposeSignUp)); ttl != 10*time.Minute || isVerified(domain.OTPPurposeUpdateUser) {
		t.Fatalf("verified ttl = %v", ttl)
	}

	if err := s.OTP().ConsumeVerified(ctx, phone, domain.OTPPurposeSignUp); err != nil || isVerified(domain.OTPPurposeSignUp) {
		t.Fatalf("ConsumeVerified: %v", err)
	}
}
