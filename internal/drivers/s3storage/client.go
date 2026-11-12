// Package s3storage stores files in an AWS S3 bucket (or an S3-compatible service such as MinIO).
package s3storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
)

const serviceName = "s3"

// cacheControl lets clients and CDNs cache objects for a year: a key is never overwritten,
// a new upload always gets a new key.
const cacheControl = "public, max-age=31536000, immutable"

type Client struct {
	api     *s3.Client
	presign *s3.PresignClient
	l       logger.Logger
	bucket  string
	baseURL string
}

func New(ctx context.Context, l logger.Logger, cfg *config.S3Config) (*Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithHTTPClient(&http.Client{
			Timeout:   cfg.Timeout,
			Transport: metrics.RoundTripper(serviceName, http.DefaultTransport),
		}),
	}

	if cfg.AccessKeyID != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, errs.Wrap(err)
	}

	api := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		}
	})

	return &Client{api: api, presign: s3.NewPresignClient(api), l: l, bucket: cfg.Bucket, baseURL: publicBaseURL(cfg)}, nil
}

// publicBaseURL is where clients read objects from, without a trailing slash.
func publicBaseURL(cfg *config.S3Config) string {
	switch {
	case cfg.PublicBaseURL != "":
		return strings.TrimRight(cfg.PublicBaseURL, "/")
	case cfg.Endpoint != "":
		return strings.TrimRight(cfg.Endpoint, "/") + "/" + cfg.Bucket
	default:
		return "https://" + cfg.Bucket + ".s3." + cfg.Region + ".amazonaws.com"
	}
}

// Put uploads data under key.
func (c *Client) Put(ctx context.Context, key, contentType string, data []byte) error {
	_, err := c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String(contentType),
		CacheControl:  aws.String(cacheControl),
	})
	if err != nil {
		c.l.Error("s3.PutObject", zap.Error(err), zap.String("key", key))

		return errs.Errf(errs.ErrConnection, "upload to storage failed: %s", err.Error())
	}

	return nil
}

// Get downloads the object under key.
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := c.api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		c.l.Error("s3.GetObject", zap.Error(err), zap.String("key", key))

		return nil, errs.Errf(errs.ErrConnection, "download from storage failed: %s", err.Error())
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, errs.Errf(errs.ErrConnection, "download from storage failed: %s", err.Error())
	}

	return data, nil
}

// Delete removes the object under key; a missing object is not an error.
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		c.l.Error("s3.DeleteObject", zap.Error(err), zap.String("key", key))

		return errs.Errf(errs.ErrConnection, "delete from storage failed: %s", err.Error())
	}

	return nil
}

// Presign returns a link that reads the object under key without credentials for ttl.
func (c *Client) Presign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)},
		s3.WithPresignExpires(ttl))
	if err != nil {
		c.l.Error("s3.PresignGetObject", zap.Error(err), zap.String("key", key))

		return "", errs.Errf(errs.ErrInternal, "signing the storage link failed: %s", err.Error())
	}

	return req.URL, nil
}

// URL returns the public URL of the object under key.
func (c *Client) URL(key string) string {
	parts := strings.Split(key, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}

	return c.baseURL + "/" + strings.Join(parts, "/")
}
