package rest

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	completionrecorder "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_recorder"
	todaylist "gitlab.com/loyihalar/birga/backend/internal/usecases/today_list"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
	"gitlab.com/loyihalar/birga/backend/pkg/logger/ginlog"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 5 * time.Minute
	writeTimeout      = 6 * time.Minute
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

type goalCreator interface {
	Execute(ctx context.Context, g domain.Goal) (domain.Goal, error)
}

type goalLister interface {
	Execute(ctx context.Context) ([]domain.Goal, error)
}

type goalGetter interface {
	Execute(ctx context.Context, id string) (domain.Goal, error)
}

type goalUpdater interface {
	Execute(ctx context.Context, id string, upd domain.GoalUpdate) (domain.Goal, error)
}

type goalDeleter interface {
	Execute(ctx context.Context, id string) error
}

// GoalUseCases groups the use cases behind /v1/goals and /v1/admin/goals.
type GoalUseCases struct {
	Creator goalCreator
	Lister  goalLister
	Getter  goalGetter
	Updater goalUpdater
	Deleter goalDeleter
}

type completionRecorder interface {
	Execute(ctx context.Context, req completionrecorder.Request) (domain.Completion, error)
}

type completionLister interface {
	Execute(ctx context.Context, userID string, f domain.CompletionFilter) ([]domain.Completion, int, error)
}

type streakGetter interface {
	Execute(ctx context.Context, userID, childID string) (domain.Streak, error)
	Days(ctx context.Context, userID, childID string, from, to time.Time) ([]domain.StreakDay, error)
}

type childCreator interface {
	Execute(ctx context.Context, userID string, pro bool, c domain.Child) (domain.Child, error)
}

type childGetter interface {
	Execute(ctx context.Context, userID, childID string) (domain.Child, error)
}

type childUpdater interface {
	Execute(ctx context.Context, userID, childID string, upd domain.ChildUpdate) (domain.Child, error)
}

type planGetter interface {
	Execute(ctx context.Context, userID, childID string) (domain.Plan, error)
	Next(ctx context.Context, userID, childID string) (domain.Plan, error)
}

type milestoneManager interface {
	Create(ctx context.Context, m domain.Milestone) (domain.Milestone, error)
	Get(ctx context.Context, id string) (domain.Milestone, error)
	Update(ctx context.Context, id string, upd domain.MilestoneUpdate) (domain.Milestone, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, f domain.MilestoneFilter) ([]domain.Milestone, int, error)
}

type childMilestones interface {
	List(ctx context.Context, userID string, pro bool, childID string, ageMonths int) (domain.ChildMilestones, error)
	Get(ctx context.Context, userID string, pro bool, id, childID string) (domain.MilestoneDetail, error)
	Mark(ctx context.Context, userID, childID, milestoneID string) error
	Unmark(ctx context.Context, userID, childID, milestoneID string) error
	Report(ctx context.Context, userID string, pro bool, childID, month string) (domain.MonthlyReport, error)
}

type todayList interface {
	Execute(ctx context.Context, userID, childID string) (domain.Today, error)
	AddExtra(ctx context.Context, req todaylist.ExtraRequest) (domain.DayExtra, error)
	ChangeDifficulty(ctx context.Context, req domain.DifficultyChange) (domain.Activity, error)
	Similar(ctx context.Context, userID string, pro bool, activityID, childID string, limit int) ([]domain.Activity, error)
}

// ChildActivityUseCases groups the use cases behind the signed-in /v1/children endpoints.
type ChildActivityUseCases struct {
	Creator  childCreator
	Getter   childGetter
	Updater  childUpdater
	Plan     planGetter
	Recorder completionRecorder
	Lister   completionLister
	Streak   streakGetter
	Today    todayList
	// Milestones serves the admin milestone CRUD; ChildMilestones the app side.
	Milestones      milestoneManager
	ChildMilestones childMilestones
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

type articleCreator interface {
	Execute(ctx context.Context, a domain.Article) (domain.Article, error)
}

type articleLister interface {
	Execute(ctx context.Context, f domain.ArticleFilter) ([]domain.Article, int, error)
}

type articleGetter interface {
	Execute(ctx context.Context, id, readerID string) (domain.Article, error)
	ForUser(ctx context.Context, userID string, pro bool, id string) (domain.Article, error)
}

type articleUpdater interface {
	Execute(ctx context.Context, id string, upd domain.ArticleUpdate) (domain.Article, error)
}

type articleDeleter interface {
	Execute(ctx context.Context, id string) error
}

type articleReader interface {
	Execute(ctx context.Context, userID, articleID string) error
}

type dailyArticle interface {
	Execute(ctx context.Context, userID string) (domain.DailyArticles, error)
	History(ctx context.Context, userID string, limit, offset int) ([]domain.DailyArticle, int, error)
}

// ArticleUseCases groups the use cases behind /v1/articles, /v1/me/articles and /v1/admin/articles.
type ArticleUseCases struct {
	Creator articleCreator
	Lister  articleLister
	Getter  articleGetter
	Updater articleUpdater
	Deleter articleDeleter
	Reader  articleReader
	Daily   dailyArticle
}

type postCreator interface {
	Execute(ctx context.Context, p domain.Post) (domain.Post, error)
}

type postLister interface {
	Execute(ctx context.Context, f domain.PostFilter) ([]domain.Post, int, error)
}

type postGetter interface {
	Execute(ctx context.Context, id string) (domain.Post, error)
	ForUser(ctx context.Context, userID string, pro bool, id string) (domain.Post, error)
}

type postUpdater interface {
	Execute(ctx context.Context, id string, upd domain.PostUpdate) (domain.Post, error)
}

type postDeleter interface {
	Execute(ctx context.Context, id string) error
}

type dailyPost interface {
	Execute(ctx context.Context, userID string) (domain.DailyPosts, error)
	History(ctx context.Context, userID string, limit, offset int) ([]domain.DailyPost, int, error)
}

// PostUseCases groups the use cases behind /v1/posts, /v1/me/posts and /v1/admin/posts.
type PostUseCases struct {
	Creator postCreator
	Lister  postLister
	Getter  postGetter
	Updater postUpdater
	Deleter postDeleter
	Daily   dailyPost
}

type topicManager interface {
	Create(ctx context.Context, t domain.Topic) (domain.Topic, error)
	List(ctx context.Context) ([]domain.Topic, error)
	Update(ctx context.Context, id string, upd domain.TopicUpdate) (domain.Topic, error)
	Delete(ctx context.Context, id string) error
}

type tipManager interface {
	Create(ctx context.Context, t domain.Tip) (domain.Tip, error)
	Get(ctx context.Context, id string, appOnly bool) (domain.Tip, error)
	Update(ctx context.Context, id string, upd domain.TipUpdate) (domain.Tip, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, f domain.TipFilter) ([]domain.Tip, int, error)
}

type dailyTip interface {
	Execute(ctx context.Context, userID string) (domain.DailyTip, error)
}

type TipUseCases struct {
	Topics topicManager
	Tips   tipManager
	Daily  dailyTip
}

type proPlan interface {
	Offer(ctx context.Context, p domain.Principal) (domain.ProOffer, error)
	Grant(ctx context.Context, userID string, until *time.Time) (domain.User, error)
	Purchase(ctx context.Context, p domain.Principal, method string) (domain.ProPurchase, error)
}

type otpSender interface {
	Execute(ctx context.Context, req domain.OTPSendRequest) (domain.OTPSendResult, error)
}

type otpVerifier interface {
	Execute(ctx context.Context, req domain.OTPVerifyRequest) error
}

// mediaLinker gives a short-lived link to an uploaded file or one of its sizes.
type mediaLinker interface {
	Link(ctx context.Context, id, size string) (link, served string, err error)
}

type mediaUploader interface {
	Execute(ctx context.Context, up domain.MediaUpload) (domain.Media, error)
	Recrop(ctx context.Context, id string, kind domain.MediaKind, crop *domain.MediaCrop) (domain.Media, error)
	MaxSize() int64
	mediaLinker
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

	goalCreator goalCreator
	goalLister  goalLister
	goalGetter  goalGetter
	goalUpdater goalUpdater
	goalDeleter goalDeleter

	childCreator       childCreator
	childGetter        childGetter
	childUpdater       childUpdater
	planGetter         planGetter
	completionRecorder completionRecorder
	completionLister   completionLister
	streakGetter       streakGetter
	today              todayList
	milestones         milestoneManager
	childMilestones    childMilestones

	userCreator userCreator
	userLister  userLister
	userGetter  userGetter
	userUpdater userUpdater
	userDeleter userDeleter

	articleCreator articleCreator
	articleLister  articleLister
	articleGetter  articleGetter
	articleUpdater articleUpdater
	articleDeleter articleDeleter
	articleReader  articleReader
	dailyArticle   dailyArticle

	postCreator postCreator
	postLister  postLister
	postGetter  postGetter
	postUpdater postUpdater
	postDeleter postDeleter
	dailyPost   dailyPost

	topics   topicManager
	tips     tipManager
	dailyTip dailyTip
	pro      proPlan

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
	// mediaLinker is nil when S3 is not configured; GET /v1/media/{id} then answers 503.
	mediaLinker mediaLinker
}

func New(cfg config.Application,
	l logger.Logger,
	health healthChecker,
	activityCreator activityCreator,
	activityLister activityLister,
	activityGetter activityGetter,
	activityEdit ActivityEditUseCases,
	goals GoalUseCases,
	childActivities ChildActivityUseCases,
	users UserUseCases,
	me MeUseCases,
	otp OTPUseCases,
	auth AuthUseCases,
	articles ArticleUseCases,
	posts PostUseCases,
	tips TipUseCases,
	pro proPlan,
	media mediaUploader,
) *Server {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Unknown routes and methods answer with the JSON envelope too, not gin's plain text.
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		fail(c, http.StatusNotFound, _errCodeNotFound, "no such endpoint: "+c.Request.Method+" "+c.Request.URL.Path)
	})
	r.NoMethod(func(c *gin.Context) {
		fail(c, http.StatusMethodNotAllowed, _errCodeBadRequest, "method "+c.Request.Method+" is not allowed on "+c.Request.URL.Path)
	})
	r.Use(
		ginlog.RequestID(),
		ginlog.Recovery(true, func(c *gin.Context) {
			fail(c, http.StatusInternalServerError, _errCodeInternalErr, errs.ErrInternal.Error())
		}),
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

		goalCreator: goals.Creator,
		goalLister:  goals.Lister,
		goalGetter:  goals.Getter,
		goalUpdater: goals.Updater,
		goalDeleter: goals.Deleter,

		childCreator:       childActivities.Creator,
		childGetter:        childActivities.Getter,
		childUpdater:       childActivities.Updater,
		planGetter:         childActivities.Plan,
		completionRecorder: childActivities.Recorder,
		completionLister:   childActivities.Lister,
		streakGetter:       childActivities.Streak,
		today:              childActivities.Today,
		milestones:         childActivities.Milestones,
		childMilestones:    childActivities.ChildMilestones,

		userCreator: users.Creator,
		userLister:  users.Lister,
		userGetter:  users.Getter,
		userUpdater: users.Updater,
		userDeleter: users.Deleter,

		articleCreator: articles.Creator,
		articleLister:  articles.Lister,
		articleGetter:  articles.Getter,
		articleUpdater: articles.Updater,
		articleDeleter: articles.Deleter,
		articleReader:  articles.Reader,
		dailyArticle:   articles.Daily,

		postCreator: posts.Creator,
		postLister:  posts.Lister,
		postGetter:  posts.Getter,
		postUpdater: posts.Updater,
		postDeleter: posts.Deleter,
		dailyPost:   posts.Daily,

		topics:   tips.Topics,
		tips:     tips.Tips,
		dailyTip: tips.Daily,
		pro:      pro,

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

	if media != nil {
		s.mediaLinker = media
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
