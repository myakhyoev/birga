package rest

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	activityrecommender "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_recommender"
	completionrecorder "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_recorder"
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

type activityUpdater interface {
	Execute(ctx context.Context, id string, upd domain.ActivityUpdate) (domain.Activity, error)
}

type activityDeleter interface {
	Execute(ctx context.Context, id string) error
}

// ActivityEditUseCases groups the use cases behind PATCH and DELETE /v1/admin/activities/{id}.
type ActivityEditUseCases struct {
	Updater activityUpdater
	Deleter activityDeleter
}

type activityRecommender interface {
	Execute(ctx context.Context, req activityrecommender.Request) (domain.Activity, error)
}

type completionRecorder interface {
	Execute(ctx context.Context, req completionrecorder.Request) (domain.Completion, error)
}

type completionLister interface {
	Execute(ctx context.Context, userID string, f domain.CompletionFilter) ([]domain.Completion, int, error)
}

type streakGetter interface {
	Execute(ctx context.Context, userID, childID string) (domain.Streak, error)
}

type childCreator interface {
	Execute(ctx context.Context, userID string, c domain.Child) (domain.Child, error)
}

type childGetter interface {
	Execute(ctx context.Context, userID, childID string) (domain.Child, error)
}

// ChildActivityUseCases groups the use cases behind the signed-in /v1/children endpoints.
type ChildActivityUseCases struct {
	Creator     childCreator
	Getter      childGetter
	Recommender activityRecommender
	Recorder    completionRecorder
	Lister      completionLister
	Streak      streakGetter
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

type profileUpdater interface {
	Execute(ctx context.Context, userID string, upd domain.UserUpdate) (domain.User, error)
}

type passwordResetter interface {
	Execute(ctx context.Context, userID, password string) (domain.TokenPair, error)
}

type childLister interface {
	Execute(ctx context.Context, userID string, f domain.ChildFilter) ([]domain.Child, int, error)
}

// MeUseCases groups the use cases behind the signed-in /v1/me endpoints that the admin user
// use cases (UserUseCases.Getter and Deleter) do not cover.
type MeUseCases struct {
	Updater  profileUpdater
	Password passwordResetter
	Children childLister
}

type otpSender interface {
	Execute(ctx context.Context, req domain.OTPSendRequest) (domain.OTPSendResult, error)
}

type otpVerifier interface {
	Execute(ctx context.Context, req domain.OTPVerifyRequest) error
}

type mediaUploader interface {
	Execute(ctx context.Context, up domain.MediaUpload) (domain.Media, error)
	MaxSize() int64
}

type signUp interface {
	Execute(ctx context.Context, req domain.SignUpRequest) (domain.TokenPair, error)
}

type tokenRefresher interface {
	Execute(ctx context.Context, refreshToken string) (domain.AccessToken, error)
}

type tokenChecker interface {
	Execute(ctx context.Context, accessToken string) (domain.Principal, error)
}

type login interface {
	Execute(ctx context.Context, username, password string) (domain.TokenPair, error)
}

type logout interface {
	Execute(ctx context.Context, userID string) error
}

type passwordForgetter interface {
	ExecuteByPhone(ctx context.Context, phone, password string) (domain.TokenPair, error)
}

// AuthUseCases groups the use cases behind the /v1/auth endpoints.
type AuthUseCases struct {
	SignUp    signUp
	Refresher tokenRefresher
	// Checker authenticates the access token on signed-in endpoints.
	Checker tokenChecker
	Login   login
	Logout  logout
	// ForgotPassword sets a new password for a signed-out user whose phone passed a reset_password check.
	ForgotPassword passwordForgetter
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
	activityUpdater activityUpdater
	activityDeleter activityDeleter

	childCreator        childCreator
	childGetter         childGetter
	activityRecommender activityRecommender
	completionRecorder  completionRecorder
	completionLister    completionLister
	streakGetter        streakGetter

	userCreator userCreator
	userLister  userLister
	userGetter  userGetter
	userUpdater userUpdater
	userDeleter userDeleter

	profileUpdater   profileUpdater
	passwordResetter passwordResetter
	childLister      childLister

	otpSender   otpSender
	otpVerifier otpVerifier

	signUp         signUp
	tokenRefresher tokenRefresher
	tokenChecker   tokenChecker
	login          login
	logout         logout
	forgotPassword passwordForgetter

	// mediaUploader is nil when S3 is not configured; POST /v1/media then answers 503.
	mediaUploader mediaUploader
}

func New(cfg config.Application,
	l logger.Logger,
	health healthChecker,
	activityCreator activityCreator,
	activityLister activityLister,
	activityGetter activityGetter,
	activityEdit ActivityEditUseCases,
	childActivities ChildActivityUseCases,
	users UserUseCases,
	me MeUseCases,
	otp OTPUseCases,
	auth AuthUseCases,
	media mediaUploader,
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
		activityUpdater: activityEdit.Updater,
		activityDeleter: activityEdit.Deleter,

		childCreator:        childActivities.Creator,
		childGetter:         childActivities.Getter,
		activityRecommender: childActivities.Recommender,
		completionRecorder:  childActivities.Recorder,
		completionLister:    childActivities.Lister,
		streakGetter:        childActivities.Streak,

		userCreator: users.Creator,
		userLister:  users.Lister,
		userGetter:  users.Getter,
		userUpdater: users.Updater,
		userDeleter: users.Deleter,

		profileUpdater:   me.Updater,
		passwordResetter: me.Password,
		childLister:      me.Children,

		otpSender:      otp.Sender,
		otpVerifier:    otp.Verifier,
		signUp:         auth.SignUp,
		tokenRefresher: auth.Refresher,
		tokenChecker:   auth.Checker,
		login:          auth.Login,
		logout:         auth.Logout,
		forgotPassword: auth.ForgotPassword,
		mediaUploader:  media,
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
			authorizationHeader,
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
