package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/httplog/v3"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
	"github.com/cwchen-twn/rigel-ledger/internal/routes"
	"github.com/cwchen-twn/rigel-ledger/web"
)

type App struct {
	cfg     *Config
	handler http.Handler
	store   *db.Store
	logger  *slog.Logger
	wg      sync.WaitGroup
}

const (
	defaultIdleTimeout    = time.Minute
	defaultReadTimeout    = 10 * time.Second
	defaultWriteTimeout   = 30 * time.Second
	defaultShutdownPeriod = 30 * time.Second
	connectTimeout        = 10 * time.Second
	sessionSweepInterval  = time.Hour
)

// OpenStore applies migrations and connects. The server and the CLI share it.
func OpenStore(cfg *Config, logger *slog.Logger) (*db.Store, error) {
	logger.Info("Applying migrations")
	if err := db.Migrate(cfg.DSN()); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	store, err := db.Open(ctx, cfg.DSN(), cfg.PgMaxConns)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return store, nil
}

func NewApp(cfg *Config, logger *slog.Logger) (*App, error) {
	store, err := OpenStore(cfg, logger)
	if err != nil {
		return nil, err
	}

	accessLogger := NewLogger(LoggerConfig{
		AppName:    cfg.AppName,
		AppVersion: cfg.AppVersion,
		AppEnv:     cfg.AppEnv,
		LogLevel:   cfg.GetLogLevel(),
		Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:       cfg.GetLogLevel(),
			ReplaceAttr: httplog.SchemaECS.Concise(cfg.IsLocalhost()).ReplaceAttr,
		}),
	})

	handler := routes.New(routes.Deps{
		Service:      ledger.NewService(store),
		Auth:         auth.NewManager(store, cfg.SessionTTL, !cfg.IsDevelopment()),
		Templates:    response.NewTemplateEngine(cfg.AppVersion, web.TemplateFiles, cfg.IsDevelopment()),
		StaticFiles:  web.StaticFiles,
		Logger:       logger,
		AccessLogger: accessLogger,
		LogLevel:     cfg.GetLogLevel(),
		AppURL:       cfg.AppURL,
		AppPort:      cfg.AppPort,
		Ready:        store.Pool.Ping,
	})

	return &App{cfg: cfg, handler: handler, store: store, logger: logger}, nil
}

// sweepSessions deletes expired sessions once an hour until ctx ends.
func (app *App) sweepSessions(ctx context.Context) {
	defer app.wg.Done()
	t := time.NewTicker(sessionSweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := app.store.DeleteExpiredSessions(ctx); err != nil {
				app.logger.Warn("Session sweep failed", "error", err)
			} else if n > 0 {
				app.logger.Info("Swept expired sessions", "count", n)
			}
		}
	}
}

func (app *App) Serve() error {
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.cfg.AppURL, app.cfg.AppPort),
		Handler:      app.handler,
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}

	bgCtx, stopBackground := context.WithCancel(context.Background())
	app.wg.Add(1)
	go app.sweepSessions(bgCtx)

	shutdownErr := make(chan error, 1)
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownPeriod)
		defer cancel()
		err := server.Shutdown(ctx)
		stopBackground()
		shutdownErr <- err
	}()

	app.logger.Info("Starting server", "addr", server.Addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		stopBackground()
		return err
	}
	if err := <-shutdownErr; err != nil {
		return err
	}
	app.wg.Wait()
	app.store.Close()
	app.logger.Info("Stopped server", "addr", server.Addr)
	return nil
}
