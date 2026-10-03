// Package smslog is a fake SMS sender for local development and tests: it writes the message
// to the log instead of sending it. Never use it in production (bootstrap refuses to).
package smslog

import (
	"context"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type Sender struct {
	l logger.Logger
}

func New(l logger.Logger) *Sender {
	return &Sender{l: l}
}

// Send logs the message, including its text (so a developer can read the code), and succeeds.
func (s *Sender) Send(ctx context.Context, messageID, phone, text string) error {
	logger.WithContext(s.l, ctx).Info("sms not sent (SMS_PROVIDER=log)",
		zap.String("message_id", messageID), zap.String("phone", phone), zap.String("text", text))

	return nil
}
