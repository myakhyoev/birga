// Package playmobile sends SMS through Play Mobile (smsxabar.uz), an SMS broker for Uzbekistan.
//
// API: POST {base}/broker-api/send with HTTP Basic auth and a JSON body of messages. A 2xx
// answer means the message was accepted; a 400 carries {"error-code", "error-description"}.
package playmobile

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
	"gitlab.com/loyihalar/birga/backend/pkg/logger/httplog"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
	"gitlab.com/loyihalar/birga/backend/pkg/remote"
)

const (
	serviceName = "playmobile"
	pathSend    = "broker-api/send"
)

type Client struct {
	api        remote.Client
	l          logger.Logger
	originator string
}

func New(l logger.Logger, cfg *config.PlayMobileConfig) *Client {
	creds := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))

	api := remote.New(cfg.BaseURL,
		remote.WithTimeout(cfg.Timeout),
		remote.WithHeader("Authorization", "Basic "+creds),
		remote.WithTransport(
			metrics.RoundTripper(serviceName,
				httplog.New(http.DefaultTransport))),
	)

	return &Client{api: api, l: l, originator: cfg.Originator}
}

type sendReq struct {
	Messages []message `json:"messages"`
}

type message struct {
	Recipient string `json:"recipient"`
	MessageID string `json:"message-id"`
	SMS       sms    `json:"sms"`
}

type sms struct {
	Originator string     `json:"originator"`
	Content    smsContent `json:"content"`
}

type smsContent struct {
	Text string `json:"text"`
}

type errorResp struct {
	Code        int    `json:"error-code"`
	Description string `json:"error-description"`
}

// Send delivers text to phone (E.164, e.g. +998901234567). messageID is our own unique id for
// the message; Play Mobile uses it in delivery reports.
func (c *Client) Send(ctx context.Context, messageID, phone, text string) error {
	l := logger.WithContext(c.l, ctx).With(zap.String("method", "playmobile.Send"), zap.String("message_id", messageID))

	req := sendReq{Messages: []message{{
		Recipient: strings.TrimPrefix(phone, "+"),
		MessageID: messageID,
		SMS:       sms{Originator: c.originator, Content: smsContent{Text: text}},
	}}}

	err := c.api.Post(ctx, nil, pathSend, req, nil)
	if err == nil {
		return nil
	}

	var se *remote.StatusError
	if !errors.As(err, &se) {
		l.Error("api.Post", zap.Error(err))

		return errs.Errf(errs.ErrConnection, "play mobile: %s", err.Error())
	}

	var er errorResp
	_ = json.Unmarshal(se.Body, &er)

	l.Error("play mobile rejected message",
		zap.Int("status", se.StatusCode), zap.Int("error_code", er.Code), zap.String("error_description", er.Description))

	if se.StatusCode >= http.StatusInternalServerError {
		return errs.Errf(errs.ErrConnection, "play mobile: status %d", se.StatusCode)
	}

	// Our input is validated before sending, so a 4xx means our account or configuration is wrong.
	return errs.Errf(errs.ErrInternal, "play mobile: status %d, error %d: %s", se.StatusCode, er.Code, er.Description)
}
