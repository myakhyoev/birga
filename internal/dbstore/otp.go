package dbstore

import (
	"context"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type otpRepo struct {
	store sqlClientProvider
}

// LockPhone takes a transaction-scoped advisory lock on phone, so concurrent send requests for
// one number check the rate limits one after another. Call it inside DBStore.InTx.
func (r *otpRepo) LockPhone(ctx context.Context, phone string) error {
	l := logger.FromCtx(ctx, "otpRepo.LockPhone")

	if _, err := r.store.sqlClientByCtx(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('otp:' || $1))`, phone); err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// SendStats returns the latest code time for phone and purpose, and how many codes were created
// since `since` for the phone (any purpose) and for the IP address.
func (r *otpRepo) SendStats(
	ctx context.Context, phone string, purpose domain.OTPPurpose, ip string, since time.Time,
) (domain.OTPSendStats, error) {
	l := logger.FromCtx(ctx, "otpRepo.SendStats")

	q := `
		SELECT
			(SELECT MAX(created_at) FROM otp_codes WHERE phone_number = $1 AND purpose = $2),
			(SELECT COUNT(*) FROM otp_codes WHERE phone_number = $1 AND created_at >= $4),
			(SELECT COUNT(*) FROM otp_codes WHERE ip_address = $3::inet AND created_at >= $4)`

	var st domain.OTPSendStats
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, phone, purpose, ip, since).
		Scan(&st.LastSentAt, &st.PhoneCount, &st.IPCount); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return domain.OTPSendStats{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return st, nil
}

// Create stores a new code. The caller sets the id (it is also the SMS message id), created_at and
// expires_at, so both come from the same clock as the rate-limit checks.
func (r *otpRepo) Create(ctx context.Context, o domain.OTP) (domain.OTP, error) {
	l := logger.FromCtx(ctx, "otpRepo.Create")

	q := `
		INSERT INTO otp_codes (id, phone_number, purpose, code_hash, ip_address, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::inet, $6, $7, $7)`

	if _, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q,
		o.ID, o.PhoneNumber, o.Purpose, o.CodeHash, o.IPAddress, o.ExpiresAt, o.CreatedAt); err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return domain.OTP{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return o, nil
}

// Delete removes a code, used when its SMS could not be sent so it does not count against the limits.
func (r *otpRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "otpRepo.Delete").With(zap.String("id", id))

	if _, err := r.store.sqlClientByCtx(ctx).Exec(ctx, `DELETE FROM otp_codes WHERE id = $1`, id); err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}
