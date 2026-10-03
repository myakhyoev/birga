package otpverifier

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeOTPs struct {
	outcome     domain.OTPVerifyOutcome
	left        int
	hash        string
	maxAttempts int
	verifiedTTL time.Duration
	called      bool
}

func (f *fakeOTPs) VerifyCode(_ context.Context, _ string, _ domain.OTPPurpose, hash string, maxAttempts int, verifiedTTL time.Duration) (domain.OTPVerifyOutcome, int, error) {
	f.called, f.hash, f.maxAttempts, f.verifiedTTL = true, hash, maxAttempts, verifiedTTL

	return f.outcome, f.left, nil
}

var cfg = config.OTPConfig{MaxVerifyAttempts: 5, VerifiedTTL: 10 * time.Minute}

func req() domain.OTPVerifyRequest {
	return domain.OTPVerifyRequest{PhoneNumber: "+998901234567", Purpose: domain.OTPPurposeSignUp, Code: " 012345 "}
}

func TestExecute_Outcomes(t *testing.T) {
	cases := []struct {
		outcome domain.OTPVerifyOutcome
		want    error
		note    string
	}{
		{domain.OTPMatched, nil, ""},
		{domain.OTPMismatch, errs.ErrValidation, "wrong code, 3 attempts left"},
		{domain.OTPNotFound, errs.ErrNotFound, errs.ErrOTPNotFound.Error()},
		{domain.OTPTooManyTries, errs.ErrRateLimited, errs.ErrOTPTooManyAttempts.Error()},
	}

	for _, tc := range cases {
		otps := &fakeOTPs{outcome: tc.outcome, left: 3}

		err := New(nil, cfg, otps).Execute(context.Background(), req())
		if !errors.Is(err, tc.want) || (err != nil && err.Error() != tc.note) {
			t.Fatalf("outcome %d: got %v, want %v %q", tc.outcome, err, tc.want, tc.note)
		}

		if otps.hash != domain.HashOTPCode("+998901234567", domain.OTPPurposeSignUp, "012345") || otps.maxAttempts != 5 || otps.verifiedTTL != 10*time.Minute {
			t.Fatalf("unexpected store call: %+v", otps)
		}
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]func(*domain.OTPVerifyRequest){
		"bad phone":   func(r *domain.OTPVerifyRequest) { r.PhoneNumber = "+79001234567" },
		"bad purpose": func(r *domain.OTPVerifyRequest) { r.Purpose = "login" },
		"short code":  func(r *domain.OTPVerifyRequest) { r.Code = "12345" },
		"letters":     func(r *domain.OTPVerifyRequest) { r.Code = "12a456" },
	}

	for name, mutate := range cases {
		r := req()
		mutate(&r)

		otps := &fakeOTPs{}
		if err := New(nil, cfg, otps).Execute(context.Background(), r); !errors.Is(err, errs.ErrValidation) || otps.called {
			t.Fatalf("%s: expected validation error before the store, got %v", name, err)
		}
	}
}
