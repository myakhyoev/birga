package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/dbstore"
	"gitlab.com/loyihalar/birga/backend/internal/gateways/rest"
	"gitlab.com/loyihalar/birga/backend/internal/redisstore"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
	"gitlab.com/loyihalar/birga/backend/pkg/metrics"
)

const gracefulDeadline = 10 * time.Second // Interval given for HTTP server to finish all requests

type App struct {
	l    *zap.Logger
	cfg  config.Application
	rest *rest.Server

	// teardown funcs run in reverse order of registration on shutdown
	teardown []func()
}

func New(cfg config.Application) *App {
	teardown := make([]func(), 0)

	// Setting UP logger
	globalLogger := logger.New(cfg.LogLevel, config.ApplicationLabel)
	teardown = append(teardown, func() { _ = logger.Cleanup() })

	globalLogger.Info("Logger initialized", zap.String("environment", cfg.Environment))

	l := logger.FromCtx(context.Background(), "bootstrap.New")

	pool, closeDB := initDB(l, cfg.Postgres)
	teardown = append(teardown, closeDB)

	// build db storage
	store := dbstore.New(pool)

	rdb, closeRedis := initRedis(l, cfg.Redis)
	teardown = append(teardown, closeRedis)

	// build redis storage (one-time codes, rate limits)
	cache := redisstore.New(rdb)

	// build integrations with other services
	drv := buildDrivers(l, cfg)

	// build usecases
	ucs := buildUseCases(l, cfg, store, cache, drv)

	httpSrv, shutdown := initREST(l, cfg, healthCheck{store, cache}, ucs)
	teardown = append(teardown, shutdown)

	return &App{
		l:        l,
		cfg:      cfg,
		rest:     httpSrv,
		teardown: teardown,
	}
}

// Run serves until ctx is cancelled, then shuts everything down gracefully.
func (app *App) Run(ctx context.Context) {
	errCh := make(chan error, 1)

	// Metrics HTTP server (separate port so it is never exposed publicly)
	go func() {
		if err := metrics.Expose(ctx, app.cfg.MetricsPort); err != nil {
			app.l.Error("metrics.Expose failed", zap.Error(err))
		}
	}()

	go func() {
		app.l.Info("HTTP server listening", zap.String("addr", app.cfg.HTTPPort))

		if err := app.rest.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		app.l.Info("shutdown signal received")
	case err := <-errCh:
		app.l.Error("HTTP server failed", zap.Error(err))
	}

	for i := len(app.teardown) - 1; i >= 0; i-- {
		app.teardown[i]()
	}
}
