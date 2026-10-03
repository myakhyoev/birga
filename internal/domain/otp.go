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

// uzPhoneRegex: an Uzbek mobile number in E.164, +998 and 9 digits. The SMS provider only delivers in Uzbekistan.
var uzPhoneRegex = regexp.MustCompile(`^\+998[0-9]{9}$`)

// IsKnown reports whether p is one of the supported purposes.
func (p OTPPurpose) IsKnown() bool {
	return p == OTPPurposeSignUp || p == OTPPurposeUpdateUser
}

// OTP is one code sent by SMS. Only the hash of the code is stored.
type OTP struct {
	ID          string
	PhoneNumber string
	Purpose     OTPPurpose
	CodeHash    string
	IPAddress   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// OTPSendRequest asks for a code to be sent to PhoneNumber. IPAddress is the end user's IP,
// used for rate limiting and audit.
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

// OTPSendStats is what the rate limits of the send-OTP endpoint look at.
type OTPSendStats struct {
	// LastSentAt is when the latest code for this phone and purpose was created; nil if never.
	LastSentAt *time.Time
	// PhoneCount and IPCount are the codes created since the window start for the phone (any purpose) and the IP.
	PhoneCount int
	IPCount    int
}

// HashOTPCode returns the stored form of code. The OTP id salts the hash, so equal codes hash differently.
func HashOTPCode(otpID, code string) string {
	sum := sha256.Sum256([]byte(otpID + ":" + code))

	return hex.EncodeToString(sum[:])
}

// IsUzbekPhoneNumber reports whether phone is an Uzbek number in E.164 (+998XXXXXXXXX).
func IsUzbekPhoneNumber(phone string) bool {
	return uzPhoneRegex.MatchString(phone)
}
