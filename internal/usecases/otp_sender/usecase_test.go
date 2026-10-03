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
	testNow = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	testCfg = config.OTPConfig{TTL: 3 * time.Minute, ResendCooldown: time.Minute, MaxPerPhoneHour: 5, MaxPerIPHour: 20}
	codeRe  = regexp.MustCompile(`\b(\d{6})\b`)
)

type fakeTx struct{}

func (fakeTx) InTx(ctx context.Context, h func(context.Context) error) error { return h(ctx) }

type fakeOTPs struct {
	stats   domain.OTPSendStats
	since   time.Time
	locked  string
	created *domain.OTP
	deleted string
}

func (f *fakeOTPs) LockPhone(_ context.Context, phone string) error {
	f.locked = phone

	return nil
}

func (f *fakeOTPs) SendStats(_ context.Context, _ string, _ domain.OTPPurpose, _ string, since time.Time) (domain.OTPSendStats, error) {
	f.since = since

	return f.stats, nil
}

func (f *fakeOTPs) Create(_ context.Context, o domain.OTP) (domain.OTP, error) {
	f.created = &o

	return o, nil
}

func (f *fakeOTPs) Delete(_ context.Context, id string) error {
	f.deleted = id

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

func newUC(otps *fakeOTPs, users fakeUsers, sms *fakeSMS) *UseCase {
	uc := New(nil, testCfg, fakeTx{}, otps, users, sms)
	uc.now = func() time.Time { return testNow }

	return uc
}

func validReq() domain.OTPSendRequest {
	return domain.OTPSendRequest{PhoneNumber: " +998901234567 ", Purpose: domain.OTPPurposeSignUp, IPAddress: "203.0.113.7"}
}

func TestExecute_Sends(t *testing.T) {
	otps, sms := &fakeOTPs{}, &fakeSMS{}

	res, err := newUC(otps, fakeUsers{}, sms).Execute(context.Background(), validReq())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if res.ExpiresIn != testCfg.TTL || res.ResendIn != testCfg.ResendCooldown {
		t.Fatalf("unexpected result: %+v", res)
	}

	o := otps.created
	if o == nil || otps.locked != "+998901234567" || o.PhoneNumber != "+998901234567" || o.IPAddress != "203.0.113.7" ||
		!o.ExpiresAt.Equal(testNow.Add(testCfg.TTL)) || !o.CreatedAt.Equal(testNow) || !otps.since.Equal(testNow.Add(-time.Hour)) {
		t.Fatalf("unexpected stored otp: %+v (locked %q, since %v)", o, otps.locked, otps.since)
	}

	m := codeRe.FindStringSubmatch(sms.text)
	if m == nil || sms.id != o.ID || sms.phone != o.PhoneNumber {
		t.Fatalf("unexpected sms: %+v", sms)
	}

	if o.CodeHash != domain.HashOTPCode(o.ID, m[1]) {
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

		if _, err := newUC(&fakeOTPs{}, fakeUsers{}, &fakeSMS{}).Execute(context.Background(), req); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("%s: expected validation error, got %v", name, err)
		}
	}
}

func TestExecute_SignUpPhoneRegistered(t *testing.T) {
	_, err := newUC(&fakeOTPs{}, fakeUsers{exists: true}, &fakeSMS{}).Execute(context.Background(), validReq())
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// update_user does not require the number to be free.
	req := validReq()
	req.Purpose = domain.OTPPurposeUpdateUser

	if _, err := newUC(&fakeOTPs{}, fakeUsers{exists: true}, &fakeSMS{}).Execute(context.Background(), req); err != nil {
		t.Fatalf("update_user: %v", err)
	}
}

func TestExecute_RateLimits(t *testing.T) {
	recent, old := testNow.Add(-20*time.Second), testNow.Add(-2*time.Minute)

	cases := map[string]domain.OTPSendStats{
		"cooldown":    {LastSentAt: &recent},
		"phone limit": {LastSentAt: &old, PhoneCount: 5},
		"ip limit":    {IPCount: 20},
	}

	for name, st := range cases {
		otps, sms := &fakeOTPs{stats: st}, &fakeSMS{}

		_, err := newUC(otps, fakeUsers{}, sms).Execute(context.Background(), validReq())
		if name == "cooldown" && (err == nil || err.Error() != "a code was sent recently, request a new one in 40 seconds") {
			t.Fatalf("cooldown message: %v", err)
		}

		if !errors.Is(err, errs.ErrRateLimited) || otps.created != nil || sms.text != "" {
			t.Fatalf("%s: expected rate limit and nothing sent, got %v", name, err)
		}
	}

	otps := &fakeOTPs{stats: domain.OTPSendStats{LastSentAt: &old, PhoneCount: 4, IPCount: 19}}
	if _, err := newUC(otps, fakeUsers{}, &fakeSMS{}).Execute(context.Background(), validReq()); err != nil {
		t.Fatalf("under limits: %v", err)
	}
}

func TestExecute_SendFailureRemovesCode(t *testing.T) {
	otps, sms := &fakeOTPs{}, &fakeSMS{err: errs.Errf(errs.ErrConnection, "down")}

	_, err := newUC(otps, fakeUsers{}, sms).Execute(context.Background(), validReq())
	if !errors.Is(err, errs.ErrConnection) || otps.deleted == "" || otps.deleted != otps.created.ID {
		t.Fatalf("expected connection error and deleted code, got %v (deleted %q)", err, otps.deleted)
	}
}

func TestGenerateCode(t *testing.T) {
	for range 100 {
		c, err := generateCode()
		if err != nil || len(c) != domain.OTPCodeLength || !codeRe.MatchString(c) {
			t.Fatalf("bad code %q: %v", c, err)
		}
	}
}
