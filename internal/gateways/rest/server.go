package rest

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
	"gitlab.com/loyihalar/birga/backend/pkg/logger/ginlog"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 2 * time.Minute
)

// routes excluded from access logs and metrics
var noisyRoutes = []string{"/swagger/*any", "/ping", "/health"}

type healthChecker interface {
	Ping(ctx context.Context) error
}

type activityCreator interface {
	Execute(ctx context.Context, a domain.Activity) (domain.Activity, error)
}

type activityLister interface {
	Execute(ctx context.Context, f domain.ActivityFilter) ([]domain.Activity, int, error)
}

type activityGetter interface {
	Execute(ctx context.Context, id string, includeUnpublished bool) (domain.Activity, error)
}

type userCreator interface {
	Execute(ctx context.Context, u domain.User) (domain.User, error)
}

type userLister interface {
	Execute(ctx context.Context, f domain.UserFilter) ([]domain.User, int, error)
}

type userGetter interface {
	Execute(ctx context.Context, id string) (domain.User, error)
}

type userUpdater interface {
	Execute(ctx context.Context, id string, upd domain.UserUpdate) (domain.User, error)
}

type userDeleter interface {
	Execute(ctx context.Context, id string) error
}

// UserUseCases groups the use cases behind the /v1/admin/users endpoints.
type UserUseCases struct {
	Creator userCreator
	Lister  userLister
	Getter  userGetter
	Updater userUpdater
	Deleter userDeleter
}

type otpSender interface {
	Execute(ctx context.Context, req domain.OTPSendRequest) (domain.OTPSendResult, error)
}

type otpVerifier interface {
	Execute(ctx context.Context, req domain.OTPVerifyRequest) error
}

// OTPUseCases groups the use cases behind the /v1/otp endpoints.
type OTPUseCases struct {
	Sender   otpSender
	Verifier otpVerifier
}

type Server struct {
	l          logger.Logger
	router     *gin.Engine
	httpServer *http.Server
	adminKey   string

	health          healthChecker
	activityCreator activityCreator
	activityLister  activityLister
	activityGetter  activityGetter

	userCreator userCreator
	userLister  userLister
	userGetter  userGetter
	userUpdater userUpdater
	userDeleter userDeleter

	otpSender   otpSender
	otpVerifier otpVerifier
}

func New(cfg config.Application,
	l logger.Logger,
	health healthChecker,
	activityCreator activityCreator,
	activityLister activityLister,
	activityGetter activityGetter,
	users UserUseCases,
	otp OTPUseCases,
) *Server {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		ginlog.RequestID(),
		ginlog.Recovery(true),
		ginlog.LogExcept(noisyRoutes, ginlog.DefaultResolver),
		metrics.Gin(resolveErrCode, noisyRoutes...),
		corsMiddleware(),
	)

	s := Server{
		l:               l,
		router:          r,
		adminKey:        cfg.AdminAPIKey,
		health:          health,
		activityCreator: activityCreator,
		activityLister:  activityLister,
		activityGetter:  activityGetter,
		userCreator:     users.Creator,
		userLister:      users.Lister,
		userGetter:      users.Getter,
		userUpdater:     users.Updater,
		userDeleter:     users.Deleter,
		otpSender:       otp.Sender,
		otpVerifier:     otp.Verifier,
	}

	s.httpServer = &http.Server{
		Addr:              cfg.HTTPPort,
		Handler:           &s,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	s.endpoints()

	return &s
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// corsMiddleware declares cors policy
func corsMiddleware(extraAllowedHeaders ...string) gin.HandlerFunc {
	var (
		allowedHeaders = append([]string{
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"Authorization",
			"Accept",
			"Origin",
			"Cache-Control",
			"X-Requested-With",
			"X-Request-Id",
			adminKeyHeader,
		}, extraAllowedHeaders...)
		allowedHeadersVal = strings.Join(allowedHeaders, ", ")
	)

	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")
		c.Header("Access-Control-Allow-Headers", allowedHeadersVal)
		c.Header("Access-Control-Expose-Headers", "X-Request-Id")
		c.Header("Access-Control-Max-Age", "3600")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)

			return
		}

		c.Next()
	}
}
