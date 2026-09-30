// Package smsservice integrates with an HTTP SMS gateway. The request/response
// contract below is a generic one; adapt paths and payloads to the provider
// you sign with (Eskiz, Play Mobile, ...).
package smsservice

import (
	"context"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
	"gitlab.com/loyihalar/birga/backend/pkg/logger/httplog"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
	"gitlab.com/loyihalar/birga/backend/pkg/remote"
)

const (
	serviceName = "sms_service"
	pathSend    = "message/sms/send"
)

type Client struct {
	api  remote.Client
	l    logger.Logger
	from string
}

func New(l logger.Logger, cfg *config.SMSServiceConfig) *Client {
	api := remote.New(cfg.BaseURL,
		remote.WithTimeout(cfg.Timeout),
		remote.WithHeader("Authorization", "Bearer "+cfg.Token),
		remote.WithTransport(
			metrics.RoundTripper(serviceName,
				httplog.New(http.DefaultTransport))),
	)

	return &Client{api: api, l: l, from: cfg.From}
}

type sendReq struct {
	MobilePhone string `json:"mobile_phone"`
	Message     string `json:"message"`
	From        string `json:"from"`
}

type sendResp struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Send delivers text to phone (E.164 without "+", e.g. 998901234567) and
// returns the provider's message id.
func (c *Client) Send(ctx context.Context, phone, text string) (string, error) {
	l := logger.WithContext(c.l, ctx).With(zap.String("method", "smsservice.Send"))

	var r sendResp
	if err := c.api.Post(ctx, &r, pathSend, sendReq{MobilePhone: phone, Message: text, From: c.from}, nil); err != nil {
		l.Error("api.Post", zap.Error(err))

		var se *remote.StatusError
		if errors.As(err, &se) && se.StatusCode < http.StatusInternalServerError {
			return "", errs.Errf(errs.ErrBadRequest, "sms gateway rejected message: status %d", se.StatusCode)
		}

		return "", errs.Errf(errs.ErrConnection, "%s", err.Error())
	}

	if r.ID == "" {
		return "", errs.Errf(errs.ErrInternal, "sms gateway returned empty message id (status %q)", r.Status)
	}

	return r.ID, nil
}
