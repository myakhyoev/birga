package bootstrap

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/dbstore"
	smsservice "gitlab.com/loyihalar/birga/backend/internal/drivers/sms_service"
	"gitlab.com/loyihalar/birga/backend/internal/gateways/rest"
	activitycreator "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_creator"
	activitygetter "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_getter"
	activitylister "gitlab.com/loyihalar/birga/backend/internal/usecases/activity_lister"
	usercreator "gitlab.com/loyihalar/birga/backend/internal/usecases/user_creator"
	userdeleter "gitlab.com/loyihalar/birga/backend/internal/usecases/user_deleter"
	usergetter "gitlab.com/loyihalar/birga/backend/internal/usecases/user_getter"
	userlister "gitlab.com/loyihalar/birga/backend/internal/usecases/user_lister"
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

// drivers - integrations with other services.
type drivers struct {
	// sms is ready for the upcoming phone sign-in (OTP) use case.
	sms *smsservice.Client
}

func buildDrivers(l *zap.Logger, cfg config.Application) *drivers {
	return &drivers{
		sms: smsservice.New(l.Named("driver.sms"), cfg.SMSService),
	}
}

// useCases - Helper structure for passing usecases into gateways
type useCases struct {
	activityCreator *activitycreator.UseCase
	activityLister  *activitylister.UseCase
	activityGetter  *activitygetter.UseCase

	userCreator *usercreator.UseCase
	userLister  *userlister.UseCase
	userGetter  *usergetter.UseCase
	userUpdater *userupdater.UseCase
	userDeleter *userdeleter.UseCase
}

func buildUseCases(l *zap.Logger, store *dbstore.DBStore, _ *drivers) *useCases {
	return &useCases{
		activityCreator: activitycreator.New(l.Named("usecase.activity_creator"), store.Activity()),
		activityLister:  activitylister.New(l.Named("usecase.activity_lister"), store.Activity()),
		activityGetter:  activitygetter.New(l.Named("usecase.activity_getter"), store.Activity()),

		userCreator: usercreator.New(l.Named("usecase.user_creator"), store.User()),
		userLister:  userlister.New(l.Named("usecase.user_lister"), store.User()),
		userGetter:  usergetter.New(l.Named("usecase.user_getter"), store.User()),
		userUpdater: userupdater.New(l.Named("usecase.user_updater"), store.User()),
		userDeleter: userdeleter.New(l.Named("usecase.user_deleter"), store.User()),
	}
}

func initREST(l *zap.Logger, cfg config.Application, store *dbstore.DBStore, ucs *useCases) (*rest.Server, func()) {
	httpSrv := rest.New(
		cfg,
		l.Named("gateway.REST"),
		store,
		ucs.activityCreator,
		ucs.activityLister,
		ucs.activityGetter,
		rest.UserUseCases{
			Creator: ucs.userCreator,
			Lister:  ucs.userLister,
			Getter:  ucs.userGetter,
			Updater: ucs.userUpdater,
			Deleter: ucs.userDeleter,
		},
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
