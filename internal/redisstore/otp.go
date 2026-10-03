package redisstore

import (
	"context"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// Keys:
//
//	birga:otp:code:<purpose>:<phone>      hash {hash, attempts}, TTL = OTP_TTL
//	birga:otp:cooldown:<purpose>:<phone>  "1", TTL = OTP_RESEND_COOLDOWN
//	birga:otp:limit:phone:<phone>         counter, TTL = limit window from the first send
//	birga:otp:limit:ip:<ip>               counter, TTL = limit window from the first send
//	birga:otp:verified:<purpose>:<phone>  "1", TTL = OTP_VERIFIED_TTL, set when a code matches
type otpRepo struct {
	rdb redis.UniversalClient
}

func codeKey(phone string, purpose domain.OTPPurpose) string {
	return keyPrefix + "otp:code:" + string(purpose) + ":" + phone
}

func cooldownKey(phone string, purpose domain.OTPPurpose) string {
	return keyPrefix + "otp:cooldown:" + string(purpose) + ":" + phone
}

func verifiedKey(phone string, purpose domain.OTPPurpose) string {
	return keyPrefix + "otp:verified:" + string(purpose) + ":" + phone
}

func phoneLimitKey(phone string) string { return keyPrefix + "otp:limit:phone:" + phone }

func ipLimitKey(ip string) string { return keyPrefix + "otp:limit:ip:" + ip }

// Script results: {status, wait in ms}.
const (
	acquireOK = iota
	acquireCooldown
	acquirePhoneLimit
	acquireIPLimit
)

// acquireScript checks the cooldown and both counters and, only if all pass, starts the cooldown
// and counts the send. Running it as one script makes check-and-count atomic across API instances.
var acquireScript = redis.NewScript(`
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 then return {1, ttl} end
if tonumber(redis.call('GET', KEYS[2]) or '0') >= tonumber(ARGV[3]) then return {2, redis.call('PTTL', KEYS[2])} end
if tonumber(redis.call('GET', KEYS[3]) or '0') >= tonumber(ARGV[4]) then return {3, redis.call('PTTL', KEYS[3])} end
redis.call('SET', KEYS[1], '1', 'PX', ARGV[1])
if redis.call('INCR', KEYS[2]) == 1 then redis.call('PEXPIRE', KEYS[2], ARGV[2]) end
if redis.call('INCR', KEYS[3]) == 1 then redis.call('PEXPIRE', KEYS[3], ARGV[2]) end
return {0, 0}
`)

// releaseScript undoes acquireScript and drops the code, for a send that failed.
var releaseScript = redis.NewScript(`
redis.call('DEL', KEYS[1], KEYS[4])
for i = 2, 3 do
  if tonumber(redis.call('GET', KEYS[i]) or '0') > 0 then redis.call('DECR', KEYS[i]) end
end
return 0
`)

// verifyScript compares the hash; a match deletes the code and marks the phone verified for
// ARGV[3] ms, a mismatch counts an attempt and deletes the code once ARGV[2] attempts are used.
// Result: {outcome, attempts left}.
var verifyScript = redis.NewScript(`
local stored = redis.call('HGET', KEYS[1], 'hash')
if not stored then return {2, 0} end
if stored == ARGV[1] then
  redis.call('DEL', KEYS[1])
  redis.call('SET', KEYS[2], '1', 'PX', ARGV[3])
  return {0, 0}
end
local left = tonumber(ARGV[2]) - redis.call('HINCRBY', KEYS[1], 'attempts', 1)
if left <= 0 then
  redis.call('DEL', KEYS[1])
  return {3, 0}
end
return {1, left}
`)

// AcquireSend is the send-OTP rate limiter. It returns an errs.ErrRateLimited error telling
// how long to wait, or nil after counting this send.
func (r *otpRepo) AcquireSend(ctx context.Context, phone string, purpose domain.OTPPurpose, ip string, lim domain.OTPLimits) error {
	l := logger.FromCtx(ctx, "otpRepo.AcquireSend")

	res, err := acquireScript.Run(ctx, r.rdb,
		[]string{cooldownKey(phone, purpose), phoneLimitKey(phone), ipLimitKey(ip)},
		lim.ResendCooldown.Milliseconds(), lim.Window.Milliseconds(), lim.MaxPerPhone, lim.MaxPerIP,
	).Int64Slice()
	if err != nil {
		l.Error("acquireScript.Run", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	wait := waitSeconds(res[1])

	switch res[0] {
	case acquireOK:
		return nil
	case acquireCooldown:
		return errs.Errf(errs.ErrRateLimited, "a code was sent recently, request a new one in %d seconds", wait)
	case acquirePhoneLimit:
		return errs.Errf(errs.ErrRateLimited, "too many codes requested for this phone number, try again in %d seconds", wait)
	case acquireIPLimit:
		return errs.Errf(errs.ErrRateLimited, "too many codes requested from this IP address, try again in %d seconds", wait)
	default:
		return errs.Errf(errs.ErrInternal, "unexpected rate limiter result %d", res[0])
	}
}

// ReleaseSend gives back what AcquireSend counted and deletes the code; used when the SMS failed.
func (r *otpRepo) ReleaseSend(ctx context.Context, phone string, purpose domain.OTPPurpose, ip string) error {
	l := logger.FromCtx(ctx, "otpRepo.ReleaseSend")

	keys := []string{cooldownKey(phone, purpose), phoneLimitKey(phone), ipLimitKey(ip), codeKey(phone, purpose)}
	if err := releaseScript.Run(ctx, r.rdb, keys).Err(); err != nil {
		l.Error("releaseScript.Run", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// SaveCode stores the hash of a new code for ttl, replacing any earlier code and its attempts.
func (r *otpRepo) SaveCode(ctx context.Context, phone string, purpose domain.OTPPurpose, hash string, ttl time.Duration) error {
	l := logger.FromCtx(ctx, "otpRepo.SaveCode")

	key := codeKey(phone, purpose)

	_, err := r.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, key, "hash", hash, "attempts", 0)
		p.PExpire(ctx, key, ttl)

		return nil
	})
	if err != nil {
		l.Error("rdb.TxPipelined", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// VerifyCode compares hash with the stored code. See domain.OTPVerifyOutcome; attemptsLeft is
// set for OTPMismatch. A match marks the phone verified for the purpose for verifiedTTL
// (see IsVerified).
func (r *otpRepo) VerifyCode(
	ctx context.Context, phone string, purpose domain.OTPPurpose, hash string, maxAttempts int, verifiedTTL time.Duration,
) (domain.OTPVerifyOutcome, int, error) {
	l := logger.FromCtx(ctx, "otpRepo.VerifyCode")

	keys := []string{codeKey(phone, purpose), verifiedKey(phone, purpose)}

	res, err := verifyScript.Run(ctx, r.rdb, keys, hash, maxAttempts, verifiedTTL.Milliseconds()).Int64Slice()
	if err != nil {
		l.Error("verifyScript.Run", zap.Error(err))

		return 0, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return domain.OTPVerifyOutcome(res[0]), int(res[1]), nil
}

// IsVerified reports whether a code for phone and purpose matched within OTP_VERIFIED_TTL and
// the mark was not consumed yet.
func (r *otpRepo) IsVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) (bool, error) {
	l := logger.FromCtx(ctx, "otpRepo.IsVerified")

	n, err := r.rdb.Exists(ctx, verifiedKey(phone, purpose)).Result()
	if err != nil {
		l.Error("rdb.Exists", zap.Error(err))

		return false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return n == 1, nil
}

// ConsumeVerified removes the verified mark, so one verification is used once.
func (r *otpRepo) ConsumeVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) error {
	l := logger.FromCtx(ctx, "otpRepo.ConsumeVerified")

	if err := r.rdb.Del(ctx, verifiedKey(phone, purpose)).Err(); err != nil {
		l.Error("rdb.Del", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// waitSeconds rounds a PTTL in milliseconds up to whole seconds (at least 1).
func waitSeconds(ms int64) int {
	return max(1, int(math.Ceil(float64(ms)/float64(time.Second.Milliseconds()))))
}
