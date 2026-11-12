package bootstrap

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/dbstore"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/drivers/playmobile"
	"gitlab.com/loyihalar/birga/backend/internal/drivers/s3storage"
	"gitlab.com/loyihalar/birga/backend/internal/drivers/smslog"
	"gitlab.com/loyihalar/birga/backend/internal/gateways/rest"
	"gitlab.com/loyihalar/birga/backend/internal/redisstore"
	"gitlab.com/loyihalar/birga/backend/internal/tokens"
	activitycreator "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_creator"
	activitydeleter "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_deleter"
	activitygetter "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_getter"
	activitylister "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_lister"
	activityupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_updater"
	articlecreator "gitlab.com/loyihalar/birga/backend/internal/usecases/article_creator"
	articledeleter "gitlab.com/loyihalar/birga/backend/internal/usecases/article_deleter"
	articlegetter "gitlab.com/loyihalar/birga/backend/internal/usecases/article_getter"
	articlelister "gitlab.com/loyihalar/birga/backend/internal/usecases/article_lister"
	articlereader "gitlab.com/loyihalar/birga/backend/internal/usecases/article_reader"
	articleupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/article_updater"
	childcreator "gitlab.com/loyihalar/birga/backend/internal/usecases/child_creator"
	childgetter "gitlab.com/loyihalar/birga/backend/internal/usecases/child_getter"
	childlister "gitlab.com/loyihalar/birga/backend/internal/usecases/child_lister"
	childmilestones "gitlab.com/loyihalar/birga/backend/internal/usecases/child_milestones"
	childupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/child_updater"
	completionlister "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_lister"
	completionrecorder "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_recorder"
	dailyarticle "gitlab.com/loyihalar/birga/backend/internal/usecases/daily_article"
	dailypost "gitlab.com/loyihalar/birga/backend/internal/usecases/daily_post"
	dailytip "gitlab.com/loyihalar/birga/backend/internal/usecases/daily_tip"
	goalcreator "gitlab.com/loyihalar/birga/backend/internal/usecases/goal_creator"
	goaldeleter "gitlab.com/loyihalar/birga/backend/internal/usecases/goal_deleter"
	goalgetter "gitlab.com/loyihalar/birga/backend/internal/usecases/goal_getter"
	goallister "gitlab.com/loyihalar/birga/backend/internal/usecases/goal_lister"
	goalupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/goal_updater"
	mediauploader "gitlab.com/loyihalar/birga/backend/internal/usecases/media_uploader"
	mediavariants "gitlab.com/loyihalar/birga/backend/internal/usecases/media_variants"
	milestonemanager "gitlab.com/loyihalar/birga/backend/internal/usecases/milestone_manager"
	otpsender "gitlab.com/loyihalar/birga/backend/internal/usecases/otp_sender"
	otpverifier "gitlab.com/loyihalar/birga/backend/internal/usecases/otp_verifier"
	passwordresetter "gitlab.com/loyihalar/birga/backend/internal/usecases/password_resetter"
	plangetter "gitlab.com/loyihalar/birga/backend/internal/usecases/plan_getter"
	postcreator "gitlab.com/loyihalar/birga/backend/internal/usecases/post_creator"
	postdeleter "gitlab.com/loyihalar/birga/backend/internal/usecases/post_deleter"
	postgetter "gitlab.com/loyihalar/birga/backend/internal/usecases/post_getter"
	postlister "gitlab.com/loyihalar/birga/backend/internal/usecases/post_lister"
	postupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/post_updater"
	proplan "gitlab.com/loyihalar/birga/backend/internal/usecases/pro_plan"
	profileupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/profile_updater"
	streakgetter "gitlab.com/loyihalar/birga/backend/internal/usecases/streak_getter"
	tipmanager "gitlab.com/loyihalar/birga/backend/internal/usecases/tip_manager"
	todaylist "gitlab.com/loyihalar/birga/backend/internal/usecases/today_list"
	tokenchecker "gitlab.com/loyihalar/birga/backend/internal/usecases/token_checker"
	tokenrefresher "gitlab.com/loyihalar/birga/backend/internal/usecases/token_refresher"
	topicmanager "gitlab.com/loyihalar/birga/backend/internal/usecases/topic_manager"
	usercreator "gitlab.com/loyihalar/birga/backend/internal/usecases/user_creator"
	userdeleter "gitlab.com/loyihalar/birga/backend/internal/usecases/user_deleter"
	usergetter "gitlab.com/loyihalar/birga/backend/internal/usecases/user_getter"
	userlister "gitlab.com/loyihalar/birga/backend/internal/usecases/user_lister"
	userlogin "gitlab.com/loyihalar/birga/backend/internal/usecases/user_login"
	userlogout "gitlab.com/loyihalar/birga/backend/internal/usecases/user_logout"
	usersignup "gitlab.com/loyihalar/birga/backend/internal/usecases/user_signup"
	userupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/user_updater"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
)

func initDB(l *zap.Logger, cfg *config.DB) (*pgxpool.Pool, func()) {
	pgxCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		l.Fatal("pgxpool.ParseConfig", zap.Error(err))
	}

	pgxCfg.ConnConfig.Tracer = metrics.NewPgxTracer()

	pgxCfg.MaxConns = cfg.MaxConns
	pgxCfg.MinConns = cfg.MinConns
	pgxCfg.MaxConnLifetime = cfg.MaxConnLifetime
	pgxCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	pgxCfg.ConnConfig.RuntimeParams["application_name"] = config.ApplicationLabel

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		l.Fatal("pgxpool.NewWithConfig", zap.Error(err))
	}

	if err := pool.Ping(ctx); err != nil {
		l.Fatal("Failed to ping Database", zap.Error(err))
	}

	metrics.MustRegister(metrics.NewPgxPoolCollector(pool))

	l.Info("Database connection established")

	return pool, func() {
		l.Info("Database pool closing...")
		pool.Close()
	}
}

func initRedis(l *zap.Logger, cfg *config.RedisConfig) (*redis.Client, func()) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		l.Fatal("redis.ParseURL", zap.Error(err))
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		l.Fatal("Failed to ping Redis", zap.Error(err))
	}

	l.Info("Redis connection established")

	return rdb, func() {
		l.Info("Redis client closing...")

		if err := rdb.Close(); err != nil {
			l.Error("rdb.Close", zap.Error(err))
		}
	}
}

type pinger interface {
	Ping(ctx context.Context) error
}

// healthCheck pings every dependency the API cannot work without.
type healthCheck []pinger

func (h healthCheck) Ping(ctx context.Context) error {
	for _, p := range h {
		if err := p.Ping(ctx); err != nil {
			return err
		}
	}

	return nil
}

// smsSender is satisfied by every SMS driver.
type smsSender interface {
	Send(ctx context.Context, messageID, phone, text string) error
}

// drivers - integrations with other services.
type drivers struct {
	sms smsSender
	// s3 is nil when S3_BUCKET is empty (media uploads disabled).
	s3 *s3storage.Client
}

func buildDrivers(l *zap.Logger, cfg config.Application) *drivers {
	return &drivers{
		sms: buildSMSSender(l, cfg),
		s3:  buildS3(l, cfg.S3),
	}
}

// buildS3 returns nil, which disables POST /v1/media, when no bucket is configured.
func buildS3(l *zap.Logger, cfg *config.S3Config) *s3storage.Client {
	if cfg.Bucket == "" {
		l.Warn("S3_BUCKET is empty: media uploads are disabled")

		return nil
	}

	if (cfg.AccessKeyID == "") != (cfg.SecretAccessKey == "") {
		l.Fatal("set both S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY, or neither to use the AWS default credentials")
	}

	c, err := s3storage.New(context.Background(), l.Named("driver.s3"), cfg)
	if err != nil {
		l.Fatal("s3storage.New", zap.Error(err))
	}

	l.Info("S3 storage configured", zap.String("bucket", cfg.Bucket), zap.String("region", cfg.Region))

	return c
}

// buildSMSSender picks the SMS driver from SMS_PROVIDER. The log driver never reaches a phone,
// so it is refused in production.
func buildSMSSender(l *zap.Logger, cfg config.Application) smsSender {
	switch cfg.SMSProvider {
	case "playmobile":
		if cfg.PlayMobile.Username == "" || cfg.PlayMobile.Password == "" {
			l.Fatal("SMS_PROVIDER=playmobile needs PLAYMOBILE_USERNAME and PLAYMOBILE_PASSWORD")
		}

		return playmobile.New(l.Named("driver.playmobile"), cfg.PlayMobile)
	case "log":
		if cfg.IsProduction() {
			l.Fatal("SMS_PROVIDER=log is not allowed in production")
		}

		l.Warn("SMS_PROVIDER=log: SMS are written to the log, not sent")

		return smslog.New(l.Named("driver.smslog"))
	default:
		l.Fatal("unknown SMS_PROVIDER", zap.String("value", cfg.SMSProvider))

		return nil
	}
}

// checkOTPDefaultCode refuses OTP_DEFAULT_CODE in production or in a wrong format, and warns
// when it is on.
func checkOTPDefaultCode(l *zap.Logger, cfg config.Application) config.OTPConfig {
	code := cfg.OTP.DefaultCode
	if code == "" {
		return *cfg.OTP
	}

	switch {
	case cfg.IsProduction():
		l.Fatal("OTP_DEFAULT_CODE is not allowed in production")
	case !domain.IsValidOTPCode(code):
		l.Fatal("OTP_DEFAULT_CODE must be 6 digits")
	}

	l.Warn("OTP_DEFAULT_CODE is set: verify accepts it for any phone number")

	return *cfg.OTP
}

// useCases - Helper structure for passing usecases into gateways
type useCases struct {
	activityCreator *activitycreator.UseCase
	activityLister  *activitylister.UseCase
	activityGetter  *activitygetter.UseCase
	activityUpdater *activityupdater.UseCase
	activityDeleter *activitydeleter.UseCase

	goalCreator *goalcreator.UseCase
	goalLister  *goallister.UseCase
	goalGetter  *goalgetter.UseCase
	goalUpdater *goalupdater.UseCase
	goalDeleter *goaldeleter.UseCase

	childCreator       *childcreator.UseCase
	childGetter        *childgetter.UseCase
	childUpdater       *childupdater.UseCase
	planGetter         *plangetter.UseCase
	childLister        *childlister.UseCase
	completionRecorder *completionrecorder.UseCase
	completionLister   *completionlister.UseCase
	streakGetter       *streakgetter.UseCase
	todayList          *todaylist.UseCase
	milestoneManager   *milestonemanager.UseCase
	childMilestones    *childmilestones.UseCase

	userCreator *usercreator.UseCase
	userLister  *userlister.UseCase
	userGetter  *usergetter.UseCase
	userUpdater *userupdater.UseCase
	userDeleter *userdeleter.UseCase

	profileUpdater   *profileupdater.UseCase
	passwordResetter *passwordresetter.UseCase

	articleCreator *articlecreator.UseCase
	articleLister  *articlelister.UseCase
	articleGetter  *articlegetter.UseCase
	articleUpdater *articleupdater.UseCase
	articleDeleter *articledeleter.UseCase
	articleReader  *articlereader.UseCase
	dailyArticle   *dailyarticle.UseCase

	postCreator *postcreator.UseCase
	postLister  *postlister.UseCase
	postGetter  *postgetter.UseCase
	postUpdater *postupdater.UseCase
	postDeleter *postdeleter.UseCase
	dailyPost   *dailypost.UseCase

	topicManager *topicmanager.UseCase
	tipManager   *tipmanager.UseCase
	dailyTip     *dailytip.UseCase
	proPlan      *proplan.UseCase

	otpSender   *otpsender.UseCase
	otpVerifier *otpverifier.UseCase

	signUp         *usersignup.UseCase
	tokenRefresher *tokenrefresher.UseCase
	tokenChecker   *tokenchecker.UseCase
	login          *userlogin.UseCase
	logout         *userlogout.UseCase

	// mediaUploader is nil when media uploads are disabled.
	mediaUploader *mediauploader.UseCase
	// mediaVariants makes the photo sizes in the background; nil when media uploads are disabled.
	mediaVariants *mediavariants.UseCase
}

func buildUseCases(l *zap.Logger, cfg config.Application, store *dbstore.DBStore, cache *redisstore.Store, drv *drivers) *useCases {
	var (
		mediaUploader *mediauploader.UseCase
		mediaVariants *mediavariants.UseCase
	)

	if drv.s3 != nil {
		mediaUploader = mediauploader.New(l.Named("usecase.media_uploader"), store.Media(), drv.s3, cfg.Media.MaxSize, cfg.S3.KeyPrefix, cfg.Media.LinkTTL)
		mediaVariants = mediavariants.New(l.Named("usecase.media_variants"), store.Media(), drv.s3, cfg.S3.KeyPrefix)
		mediaUploader.OnUpload(mediaVariants.Wake)
	}

	jwt, err := tokens.New(*cfg.JWT)
	if err != nil {
		l.Fatal("tokens.New", zap.Error(err))
	}

	var (
		userGetter  = usergetter.New(l.Named("usecase.user_getter"), store.User())
		userUpdater = userupdater.New(l.Named("usecase.user_updater"), store.User())
		planGetter  = plangetter.New(l.Named("usecase.plan_getter"), store, store.Child(), store.Plan())
	)

	return &useCases{
		activityCreator: activitycreator.New(l.Named("usecase.activity_creator"), store.Activity(), store.Goal()),
		activityLister:  activitylister.New(l.Named("usecase.activity_lister"), store.Activity()),
		activityGetter:  activitygetter.New(l.Named("usecase.activity_getter"), store.Activity()),
		activityUpdater: activityupdater.New(l.Named("usecase.activity_updater"), store.Activity(), store.Goal()),
		activityDeleter: activitydeleter.New(l.Named("usecase.activity_deleter"), store.Activity()),

		goalCreator: goalcreator.New(l.Named("usecase.goal_creator"), store.Goal()),
		goalLister:  goallister.New(l.Named("usecase.goal_lister"), store.Goal()),
		goalGetter:  goalgetter.New(l.Named("usecase.goal_getter"), store.Goal()),
		goalUpdater: goalupdater.New(l.Named("usecase.goal_updater"), store.Goal()),
		goalDeleter: goaldeleter.New(l.Named("usecase.goal_deleter"), store.Goal()),

		childCreator: childcreator.New(l.Named("usecase.child_creator"), store, store.Child(), store.Goal(), planGetter),
		childGetter:  childgetter.New(l.Named("usecase.child_getter"), store.Child()),
		childUpdater: childupdater.New(l.Named("usecase.child_updater"), store.Child(), store.Goal()),
		planGetter:   planGetter,
		childLister:  childlister.New(l.Named("usecase.child_lister"), store.Child()),
		completionRecorder: completionrecorder.New(l.Named("usecase.completion_recorder"),
			store, store.Child(), store.Activity(), store.Completion(), store.Plan()),
		completionLister: completionlister.New(l.Named("usecase.completion_lister"), store.Child(), store.Completion()),
		streakGetter:     streakgetter.New(l.Named("usecase.streak_getter"), store.Child(), store.Plan()),
		todayList: todaylist.New(l.Named("usecase.today_list"),
			store.Child(), planGetter, store.Activity(), store.Extra(), store.Goal(), store.Plan()),
		milestoneManager: milestonemanager.New(l.Named("usecase.milestone_manager"), store.Milestone(), store.Activity()),
		childMilestones: childmilestones.New(l.Named("usecase.child_milestones"),
			store.Child(), store.Milestone(), store.Activity(), store.Completion()),

		userCreator: usercreator.New(l.Named("usecase.user_creator"), store.User()),
		userLister:  userlister.New(l.Named("usecase.user_lister"), store.User()),
		userGetter:  userGetter,
		userUpdater: userUpdater,
		userDeleter: userdeleter.New(l.Named("usecase.user_deleter"), store.User()),

		profileUpdater: profileupdater.New(l.Named("usecase.profile_updater"), cache.OTP(), userGetter, userUpdater),

		articleCreator: articlecreator.New(l.Named("usecase.article_creator"), store.Article(), store.Goal()),
		articleLister:  articlelister.New(l.Named("usecase.article_lister"), store.Article()),
		articleGetter:  articlegetter.New(l.Named("usecase.article_getter"), store.Article()),
		articleUpdater: articleupdater.New(l.Named("usecase.article_updater"), store.Article(), store.Goal()),
		articleDeleter: articledeleter.New(l.Named("usecase.article_deleter"), store.Article()),
		articleReader:  articlereader.New(l.Named("usecase.article_reader"), store.Article()),
		dailyArticle:   dailyarticle.New(l.Named("usecase.daily_article"), store, store.Article()),

		topicManager: topicmanager.New(l.Named("usecase.topic_manager"), store.Topic()),
		tipManager:   tipmanager.New(l.Named("usecase.tip_manager"), store.Tip(), store.Topic()),
		dailyTip:     dailytip.New(l.Named("usecase.daily_tip"), store, store.Tip()),
		proPlan:      proplan.New(l.Named("usecase.pro_plan"), store.User(), cfg.ProPriceUZS),

		postCreator: postcreator.New(l.Named("usecase.post_creator"), store.Post(), store.Goal(), store.Media()),
		postLister:  postlister.New(l.Named("usecase.post_lister"), store.Post()),
		postGetter:  postgetter.New(l.Named("usecase.post_getter"), store.Post()),
		postUpdater: postupdater.New(l.Named("usecase.post_updater"), store.Post(), store.Goal(), store.Media()),
		postDeleter: postdeleter.New(l.Named("usecase.post_deleter"), store.Post()),
		dailyPost:   dailypost.New(l.Named("usecase.daily_post"), store, store.Post()),

		otpSender:   otpsender.New(l.Named("usecase.otp_sender"), *cfg.OTP, cache.OTP(), store.User(), drv.sms),
		otpVerifier: otpverifier.New(l.Named("usecase.otp_verifier"), checkOTPDefaultCode(l, cfg), cache.OTP()),

		signUp:         usersignup.New(l.Named("usecase.user_signup"), store, store.User(), store.Auth(), jwt),
		tokenRefresher: tokenrefresher.New(l.Named("usecase.token_refresher"), jwt, store.Auth()),
		tokenChecker:   tokenchecker.New(l.Named("usecase.token_checker"), jwt, store.Auth()),
		login:          userlogin.New(l.Named("usecase.user_login"), store.Auth(), jwt),
		logout:         userlogout.New(l.Named("usecase.user_logout"), store.Auth()),
		passwordResetter: passwordresetter.New(l.Named("usecase.password_resetter"),
			cache.OTP(), store.User(), store.Auth(), jwt),

		mediaUploader: mediaUploader,
		mediaVariants: mediaVariants,
	}
}

func initREST(l *zap.Logger, cfg config.Application, health pinger, ucs *useCases) (*rest.Server, func()) {
	// A nil *UseCase inside a non-nil interface would look enabled to the server, so pass a true nil.
	var media interface {
		Execute(ctx context.Context, up domain.MediaUpload) (domain.Media, error)
		Recrop(ctx context.Context, id string, kind domain.MediaKind, crop *domain.MediaCrop) (domain.Media, error)
		MaxSize() int64
		Link(ctx context.Context, id, size string) (link, served string, err error)
	}
	if ucs.mediaUploader != nil {
		media = ucs.mediaUploader
	}

	httpSrv := rest.New(
		cfg,
		l.Named("gateway.REST"),
		health,
		ucs.activityCreator,
		ucs.activityLister,
		ucs.activityGetter,
		rest.ActivityEditUseCases{
			Updater: ucs.activityUpdater,
			Deleter: ucs.activityDeleter,
		},
		rest.GoalUseCases{
			Creator: ucs.goalCreator,
			Lister:  ucs.goalLister,
			Getter:  ucs.goalGetter,
			Updater: ucs.goalUpdater,
			Deleter: ucs.goalDeleter,
		},
		rest.ChildActivityUseCases{
			Creator:  ucs.childCreator,
			Getter:   ucs.childGetter,
			Updater:  ucs.childUpdater,
			Plan:     ucs.planGetter,
			Recorder: ucs.completionRecorder,
			Lister:   ucs.completionLister,
			Streak:   ucs.streakGetter,
			Today:    ucs.todayList,

			Milestones:      ucs.milestoneManager,
			ChildMilestones: ucs.childMilestones,
		},
		rest.UserUseCases{
			Creator: ucs.userCreator,
			Lister:  ucs.userLister,
			Getter:  ucs.userGetter,
			Updater: ucs.userUpdater,
			Deleter: ucs.userDeleter,
		},
		rest.MeUseCases{
			Updater:  ucs.profileUpdater,
			Password: ucs.passwordResetter,
			Children: ucs.childLister,
		},
		rest.OTPUseCases{
			Sender:   ucs.otpSender,
			Verifier: ucs.otpVerifier,
		},
		rest.AuthUseCases{
			SignUp:    ucs.signUp,
			Refresher: ucs.tokenRefresher,
			Checker:   ucs.tokenChecker,
			Login:     ucs.login,
			Logout:    ucs.logout,
			// The same use case serves PUT /v1/me/password (signed in) and forgot password (signed out).
			ForgotPassword: ucs.passwordResetter,
		},
		rest.ArticleUseCases{
			Creator: ucs.articleCreator,
			Lister:  ucs.articleLister,
			Getter:  ucs.articleGetter,
			Updater: ucs.articleUpdater,
			Deleter: ucs.articleDeleter,
			Reader:  ucs.articleReader,
			Daily:   ucs.dailyArticle,
		},
		rest.PostUseCases{
			Creator: ucs.postCreator,
			Lister:  ucs.postLister,
			Getter:  ucs.postGetter,
			Updater: ucs.postUpdater,
			Deleter: ucs.postDeleter,
			Daily:   ucs.dailyPost,
		},
		rest.TipUseCases{
			Topics: ucs.topicManager,
			Tips:   ucs.tipManager,
			Daily:  ucs.dailyTip,
		},
		ucs.proPlan,
		media,
	)

	return httpSrv, func() {
		l.Info("HTTP is shutting down")

		ctxShutDown, cancel := context.WithTimeout(context.Background(), gracefulDeadline)
		defer cancel()

		if err := httpSrv.Shutdown(ctxShutDown); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.Error("httpSrv.Shutdown", zap.Error(err))

			return
		}

		l.Info("HTTP is shut down")
	}
}
