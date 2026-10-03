package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"time"
)

// OTPPurpose says why a one-time code is sent. A code is only valid for the purpose it was sent for.
type OTPPurpose string

const (
	OTPPurposeSignUp     OTPPurpose = "sign_up"
	OTPPurposeUpdateUser OTPPurpose = "update_user"
)

// OTPCodeLength is the number of digits in a one-time code.
const OTPCodeLength = 6

var (
	// uzPhoneRegex: an Uzbek mobile number in E.164, +998 and 9 digits. The SMS provider only delivers in Uzbekistan.
	uzPhoneRegex = regexp.MustCompile(`^\+998[0-9]{9}$`)
	// otpCodeRegex: exactly OTPCodeLength digits.
	otpCodeRegex = regexp.MustCompile(`^[0-9]{6}$`)
)

// IsKnown reports whether p is one of the supported purposes.
func (p OTPPurpose) IsKnown() bool {
	return p == OTPPurposeSignUp || p == OTPPurposeUpdateUser
}

// OTPSendRequest asks for a code to be sent to PhoneNumber. IPAddress is the end user's IP,
// used for rate limiting.
type OTPSendRequest struct {
	PhoneNumber string
	Purpose     OTPPurpose
	IPAddress   string
}

// OTPSendResult tells the client how long the code lives and when it may ask for another one.
type OTPSendResult struct {
	ExpiresIn time.Duration
	ResendIn  time.Duration
}

// OTPVerifyRequest checks Code against the code last sent to PhoneNumber for Purpose.
type OTPVerifyRequest struct {
	PhoneNumber string
	Purpose     OTPPurpose
	Code        string
}

// OTPLimits are the send limits the rate limiter enforces.
type OTPLimits struct {
	ResendCooldown time.Duration // one code per phone and purpose per cooldown
	Window         time.Duration // window of the counters below
	MaxPerPhone    int           // codes per phone (any purpose) per window
	MaxPerIP       int           // codes per IP per window
}

// OTPVerifyOutcome is the result of comparing a code with the stored one.
type OTPVerifyOutcome int

const (
	OTPMatched      OTPVerifyOutcome = iota // code matched; the stored code is deleted
	OTPMismatch                             // wrong code; attempts left
	OTPNotFound                             // no code: never sent, expired or already used
	OTPTooManyTries                         // wrong code and no attempts left; the stored code is deleted
)

// HashOTPCode returns the stored form of a code. Phone and purpose salt the hash, so the plain
// code never reaches the store.
func HashOTPCode(phone string, purpose OTPPurpose, code string) string {
	sum := sha256.Sum256([]byte(phone + ":" + string(purpose) + ":" + code))

	return hex.EncodeToString(sum[:])
}

// IsUzbekPhoneNumber reports whether phone is an Uzbek number in E.164 (+998XXXXXXXXX).
func IsUzbekPhoneNumber(phone string) bool {
	return uzPhoneRegex.MatchString(phone)
}

// IsValidOTPCode reports whether code has the one-time code format.
func IsValidOTPCode(code string) bool {
	return otpCodeRegex.MatchString(code)
}
