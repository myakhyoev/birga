package config

import "time"

const ApplicationLabel = "birga_backend"

type Application struct {
	Environment string `env:"ENVIRONMENT, default=development"` // development, staging, production
	LogLevel    string `env:"LOG_LEVEL, default=info"`
	HTTPPort    string `env:"HTTP_PORT, default=:8080"`
	MetricsPort string `env:"METRICS_PORT, default=:9090"`

	// AdminAPIKey protects content-management endpoints (X-Admin-Key header).
	// When empty, those endpoints are disabled.
	AdminAPIKey string `env:"ADMIN_API_KEY"`

	Postgres *DB `env:", prefix=POSTGRES_"`

	// SMSProvider picks the SMS driver: "playmobile" sends real SMS, "log" only logs them
	// (local development; refused in production).
	SMSProvider string `env:"SMS_PROVIDER, default=log"`

	PlayMobile *PlayMobileConfig `env:", prefix=PLAYMOBILE_"`

	Redis *RedisConfig `env:", prefix=REDIS_"`

	OTP *OTPConfig `env:", prefix=OTP_"`

	S3 *S3Config `env:", prefix=S3_"`

	Media *MediaConfig `env:", prefix=MEDIA_"`
}

func (c *Application) IsProduction() bool {
	return c.Environment == "production"
}

type DB struct {
	// URL is a full connection string, e.g. postgres://user:pass@localhost:5432/birga?sslmode=disable
	URL string `env:"URL, required"`

	MaxConns        int32         `env:"MAX_CONNS, default=25"`
	MinConns        int32         `env:"MIN_CONNS, default=2"`
	MaxConnLifetime time.Duration `env:"MAX_CONN_LIFETIME, default=30m"`
	MaxConnIdleTime time.Duration `env:"MAX_CONN_IDLE_TIME, default=5m"`
	ConnectTimeout  time.Duration `env:"CONNECT_TIMEOUT, default=5s"`
}

// PlayMobileConfig configures the Play Mobile (smsxabar.uz) SMS driver. Username, password
// and originator come from the Play Mobile contract.
type PlayMobileConfig struct {
	BaseURL    string        `env:"BASE_URL, default=https://send.smsxabar.uz"`
	Username   string        `env:"USERNAME"`
	Password   string        `env:"PASSWORD"`
	Originator string        `env:"ORIGINATOR, default=3700"`
	Timeout    time.Duration `env:"TIMEOUT, default=10s"`
}

// OTPConfig controls one-time codes sent by SMS and the limits on sending them.
type OTPConfig struct {
	TTL             time.Duration `env:"TTL, default=2m"`
	ResendCooldown  time.Duration `env:"RESEND_COOLDOWN, default=1m"`
	MaxPerPhoneHour int           `env:"MAX_PER_PHONE_HOUR, default=5"`
	MaxPerIPHour    int           `env:"MAX_PER_IP_HOUR, default=20"`
	// MaxVerifyAttempts wrong codes delete the code; the user must request a new one.
	MaxVerifyAttempts int `env:"MAX_VERIFY_ATTEMPTS, default=5"`
}

// RedisConfig configures the Redis client that holds one-time codes and rate limits.
type RedisConfig struct {
	// URL is a redis:// or rediss:// URL, e.g. redis://:password@localhost:6379/0
	URL            string        `env:"URL, default=redis://localhost:6379/0"`
	ConnectTimeout time.Duration `env:"CONNECT_TIMEOUT, default=5s"`
}

// S3Config configures the AWS S3 bucket that stores uploaded media. With Bucket empty, uploads are
// disabled. With AccessKeyID empty, credentials come from the AWS default chain (AWS_* env vars,
// shared config, an IAM role).
type S3Config struct {
	Bucket          string `env:"BUCKET"`
	Region          string `env:"REGION, default=eu-central-1"`
	AccessKeyID     string `env:"ACCESS_KEY_ID"`
	SecretAccessKey string `env:"SECRET_ACCESS_KEY"`
	// Endpoint overrides the AWS endpoint for S3-compatible storage (MinIO, LocalStack); it also
	// switches to path-style URLs.
	Endpoint string `env:"ENDPOINT"`
	// PublicBaseURL is the prefix of the URLs returned to clients, e.g. a CloudFront domain.
	// Empty means https://<bucket>.s3.<region>.amazonaws.com.
	PublicBaseURL string `env:"PUBLIC_BASE_URL"`
	// KeyPrefix is put in front of every object key, e.g. "staging/".
	KeyPrefix string        `env:"KEY_PREFIX"`
	Timeout   time.Duration `env:"TIMEOUT, default=30s"`
}

// MediaConfig limits uploaded media.
type MediaConfig struct {
	// MaxSize is the largest accepted file, in bytes, after base64 decoding.
	MaxSize int64 `env:"MAX_SIZE, default=5242880"`
}
