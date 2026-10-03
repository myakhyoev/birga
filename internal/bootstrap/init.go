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
	activityrecommender "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_recommender"
	activityupdater "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_updater"
	completionlister "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_lister"
	completionrecorder "gitlab.com/loyihalar/birga/backend/internal/usecases/completion_recorder"
	mediauploader "gitlab.com/loyihalar/birga/backend/internal/usecases/media_uploader"
	otpsender "gitlab.com/loyihalar/birga/backend/internal/usecases/otp_sender"
	otpverifier "gitlab.com/loyihalar/birga/backend/internal/usecases/otp_verifier"
	streakgetter "gitlab.com/loyihalar/birga/backend/internal/usecases/streak_getter"
	tokenchecker "gitlab.com/loyihalar/birga/backend/internal/usecases/token_checker"
	tokenrefresher "gitlab.com/loyihalar/birga/backend/internal/usecases/token_refresher"
	usercreator "gitlab.com/loyihalar/birga/backend/internal/usecases/user_creator"
	userdeleter "gitlab.com/loyihalar/birga/backend/internal/usecases/user_deleter"
	usergetter "gitlab.com/loyihalar/birga/backend/internal/usecases/user_getter"
	userlister "gitlab.com/loyihalar/birga/backend/internal/usecases/user_lister"
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

	activityRecommender *activityrecommender.UseCase
	completionRecorder  *completionrecorder.UseCase
	completionLister    *completionlister.UseCase
	streakGetter        *streakgetter.UseCase

	userCreator *usercreator.UseCase
	userLister  *userlister.UseCase
	userGetter  *usergetter.UseCase
	userUpdater *userupdater.UseCase
	userDeleter *userdeleter.UseCase

	otpSender   *otpsender.UseCase
	otpVerifier *otpverifier.UseCase

	signUp         *usersignup.UseCase
	tokenRefresher *tokenrefresher.UseCase
	tokenChecker   *tokenchecker.UseCase

	// mediaUploader is nil when media uploads are disabled.
	mediaUploader *mediauploader.UseCase
}

func buildUseCases(l *zap.Logger, cfg config.Application, store *dbstore.DBStore, cache *redisstore.Store, drv *drivers) *useCases {
	var mediaUploader *mediauploader.UseCase
	if drv.s3 != nil {
		mediaUploader = mediauploader.New(l.Named("usecase.media_uploader"), store.Media(), drv.s3, cfg.Media.MaxSize, cfg.S3.KeyPrefix)
	}

	jwt, err := tokens.New(*cfg.JWT)
	if err != nil {
		l.Fatal("tokens.New", zap.Error(err))
	}

	return &useCases{
		activityCreator: activitycreator.New(l.Named("usecase.activity_creator"), store.Activity()),
		activityLister:  activitylister.New(l.Named("usecase.activity_lister"), store.Activity()),
		activityGetter:  activitygetter.New(l.Named("usecase.activity_getter"), store.Activity()),
		activityUpdater: activityupdater.New(l.Named("usecase.activity_updater"), store.Activity()),
		activityDeleter: activitydeleter.New(l.Named("usecase.activity_deleter"), store.Activity()),

		activityRecommender: activityrecommender.New(l.Named("usecase.activity_recommender"), store.Child(), store.Activity()),
		completionRecorder: completionrecorder.New(l.Named("usecase.completion_recorder"),
			store.Child(), store.Activity(), store.Completion()),
		completionLister: completionlister.New(l.Named("usecase.completion_lister"), store.Child(), store.Completion()),
		streakGetter:     streakgetter.New(l.Named("usecase.streak_getter"), store.Child(), store.Completion()),

		userCreator: usercreator.New(l.Named("usecase.user_creator"), store.User()),
		userLister:  userlister.New(l.Named("usecase.user_lister"), store.User()),
		userGetter:  usergetter.New(l.Named("usecase.user_getter"), store.User()),
		userUpdater: userupdater.New(l.Named("usecase.user_updater"), store.User()),
		userDeleter: userdeleter.New(l.Named("usecase.user_deleter"), store.User()),

		otpSender:   otpsender.New(l.Named("usecase.otp_sender"), *cfg.OTP, cache.OTP(), store.User(), drv.sms),
		otpVerifier: otpverifier.New(l.Named("usecase.otp_verifier"), checkOTPDefaultCode(l, cfg), cache.OTP()),

		signUp:         usersignup.New(l.Named("usecase.user_signup"), cache.OTP(), store, store.User(), store.Auth(), jwt),
		tokenRefresher: tokenrefresher.New(l.Named("usecase.token_refresher"), jwt, store.Auth()),
		tokenChecker:   tokenchecker.New(l.Named("usecase.token_checker"), jwt, store.Auth()),

		mediaUploader: mediaUploader,
	}
}

func initREST(l *zap.Logger, cfg config.Application, health pinger, ucs *useCases) (*rest.Server, func()) {
	// A nil *UseCase inside a non-nil interface would look enabled to the server, so pass a true nil.
	var media interface {
		Execute(ctx context.Context, up domain.MediaUpload) (domain.Media, error)
		MaxSize() int64
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
		rest.ChildActivityUseCases{
			Recommender: ucs.activityRecommender,
			Recorder:    ucs.completionRecorder,
			Lister:      ucs.completionLister,
			Streak:      ucs.streakGetter,
		},
		rest.UserUseCases{
			Creator: ucs.userCreator,
			Lister:  ucs.userLister,
			Getter:  ucs.userGetter,
			Updater: ucs.userUpdater,
			Deleter: ucs.userDeleter,
		},
		rest.OTPUseCases{
			Sender:   ucs.otpSender,
			Verifier: ucs.otpVerifier,
		},
		rest.AuthUseCases{
			SignUp:    ucs.signUp,
			Refresher: ucs.tokenRefresher,
			Checker:   ucs.tokenChecker,
		},
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
