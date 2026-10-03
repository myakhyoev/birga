package rest

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeOTPSender struct {
	got domain.OTPSendRequest
	err error
}

func (f *fakeOTPSender) Execute(_ context.Context, req domain.OTPSendRequest) (domain.OTPSendResult, error) {
	f.got = req

	return domain.OTPSendResult{ExpiresIn: 3 * time.Minute, ResendIn: time.Minute}, f.err
}

func TestSendOTP(t *testing.T) {
	s, d := newTestServer("")

	code, r := do(t, s, http.MethodPost, "/v1/otp/send",
		`{"phone_number": "+998901234567", "purpose": "update_user", "ip_address": "203.0.113.7"}`, nil)
	if code != http.StatusOK {
		t.Fatalf("got %d %+v", code, r)
	}

	want := domain.OTPSendRequest{PhoneNumber: "+998901234567", Purpose: domain.OTPPurposeUpdateUser, IPAddress: "203.0.113.7"}
	if d.otp.got != want {
		t.Fatalf("request = %+v, want %+v", d.otp.got, want)
	}

	data, _ := r.Data.(map[string]any)
	if data["expires_in"] != float64(180) || data["resend_in"] != float64(60) {
		t.Fatalf("unexpected data: %+v", r.Data)
	}
}

func TestSendOTP_Errors(t *testing.T) {
	s, d := newTestServer("")

	if code, r := do(t, s, http.MethodPost, "/v1/otp/send", `{`, nil); code != http.StatusBadRequest {
		t.Fatalf("bad json: %d %+v", code, r)
	}

	d.otp.err = errs.Errf(errs.ErrRateLimited, "wait")

	code, r := do(t, s, http.MethodPost, "/v1/otp/send", `{}`, nil)
	if code != http.StatusTooManyRequests || r.ErrorCode != _errCodeRateLimited || r.ErrorNote != "wait" {
		t.Fatalf("rate limited: %d %+v", code, r)
	}
}
