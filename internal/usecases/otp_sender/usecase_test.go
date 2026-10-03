package otpsender

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

var (
	testCfg = config.OTPConfig{TTL: 2 * time.Minute, ResendCooldown: time.Minute, MaxPerPhoneHour: 5, MaxPerIPHour: 20}
	codeRe  = regexp.MustCompile(`\b(\d{6})\b`)
)

type fakeOTPs struct {
	acquireErr error
	limits     domain.OTPLimits
	ip         string
	hash       string
	ttl        time.Duration
	released   bool
}

func (f *fakeOTPs) AcquireSend(_ context.Context, _ string, _ domain.OTPPurpose, ip string, lim domain.OTPLimits) error {
	f.ip, f.limits = ip, lim

	return f.acquireErr
}

func (f *fakeOTPs) ReleaseSend(context.Context, string, domain.OTPPurpose, string) error {
	f.released = true

	return nil
}

func (f *fakeOTPs) SaveCode(_ context.Context, _ string, _ domain.OTPPurpose, hash string, ttl time.Duration) error {
	f.hash, f.ttl = hash, ttl

	return nil
}

type fakeUsers struct{ exists bool }

func (f fakeUsers) ExistsByPhone(context.Context, string) (bool, error) { return f.exists, nil }

type fakeSMS struct {
	id, phone, text string
	err             error
}

func (f *fakeSMS) Send(_ context.Context, id, phone, text string) error {
	f.id, f.phone, f.text = id, phone, text

	return f.err
}

func validReq() domain.OTPSendRequest {
	return domain.OTPSendRequest{PhoneNumber: " +998901234567 ", Purpose: domain.OTPPurposeSignUp, IPAddress: " 2001:DB8::1 "}
}

func TestExecute_Sends(t *testing.T) {
	otps, sms := &fakeOTPs{}, &fakeSMS{}

	res, err := New(nil, testCfg, otps, fakeUsers{}, sms).Execute(context.Background(), validReq())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if res.ExpiresIn != testCfg.TTL || res.ResendIn != testCfg.ResendCooldown {
		t.Fatalf("unexpected result: %+v", res)
	}

	wantLimits := domain.OTPLimits{ResendCooldown: time.Minute, Window: time.Hour, MaxPerPhone: 5, MaxPerIP: 20}
	if otps.limits != wantLimits || otps.ip != "2001:db8::1" || otps.ttl != testCfg.TTL || otps.released {
		t.Fatalf("unexpected store calls: %+v", otps)
	}

	m := codeRe.FindStringSubmatch(sms.text)
	if m == nil || sms.id == "" || sms.phone != "+998901234567" {
		t.Fatalf("unexpected sms: %+v", sms)
	}

	if otps.hash != domain.HashOTPCode("+998901234567", domain.OTPPurposeSignUp, m[1]) {
		t.Fatalf("stored hash does not match the sent code")
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]func(*domain.OTPSendRequest){
		"no phone":       func(r *domain.OTPSendRequest) { r.PhoneNumber = "" },
		"foreign phone":  func(r *domain.OTPSendRequest) { r.PhoneNumber = "+79001234567" },
		"local phone":    func(r *domain.OTPSendRequest) { r.PhoneNumber = "901234567" },
		"bad purpose":    func(r *domain.OTPSendRequest) { r.Purpose = "login" },
		"no purpose":     func(r *domain.OTPSendRequest) { r.Purpose = "" },
		"no ip":          func(r *domain.OTPSendRequest) { r.IPAddress = "" },
		"bad ip":         func(r *domain.OTPSendRequest) { r.IPAddress = "300.1.1.1" },
		"hostname as ip": func(r *domain.OTPSendRequest) { r.IPAddress = "example.com" },
	}

	for name, mutate := range cases {
		req := validReq()
		mutate(&req)

		if _, err := New(nil, testCfg, &fakeOTPs{}, fakeUsers{}, &fakeSMS{}).Execute(context.Background(), req); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("%s: expected validation error, got %v", name, err)
		}
	}
}

func TestExecute_SignUpPhoneRegistered(t *testing.T) {
	_, err := New(nil, testCfg, &fakeOTPs{}, fakeUsers{exists: true}, &fakeSMS{}).Execute(context.Background(), validReq())
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// update_user does not require the number to be free.
	req := validReq()
	req.Purpose = domain.OTPPurposeUpdateUser

	if _, err := New(nil, testCfg, &fakeOTPs{}, fakeUsers{exists: true}, &fakeSMS{}).Execute(context.Background(), req); err != nil {
		t.Fatalf("update_user: %v", err)
	}
}

func TestExecute_RateLimited(t *testing.T) {
	otps, sms := &fakeOTPs{acquireErr: errs.Errf(errs.ErrRateLimited, "wait")}, &fakeSMS{}

	_, err := New(nil, testCfg, otps, fakeUsers{}, sms).Execute(context.Background(), validReq())
	if !errors.Is(err, errs.ErrRateLimited) || otps.hash != "" || sms.text != "" || otps.released {
		t.Fatalf("expected rate limit and nothing stored or sent, got %v", err)
	}
}

func TestExecute_SendFailureReleases(t *testing.T) {
	otps, sms := &fakeOTPs{}, &fakeSMS{err: errs.Errf(errs.ErrConnection, "down")}

	_, err := New(nil, testCfg, otps, fakeUsers{}, sms).Execute(context.Background(), validReq())
	if !errors.Is(err, errs.ErrConnection) || !otps.released {
		t.Fatalf("expected connection error and released send, got %v (released %v)", err, otps.released)
	}
}

func TestGenerateCode(t *testing.T) {
	for range 100 {
		c, err := generateCode()
		if err != nil || !domain.IsValidOTPCode(c) {
			t.Fatalf("bad code %q: %v", c, err)
		}
	}
}
