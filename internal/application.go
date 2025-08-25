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

	"github.com/cwc1222/rigelledger/internal/auth"
	"github.com/cwc1222/rigelledger/internal/response"
	"github.com/cwc1222/rigelledger/internal/routes"
	"github.com/cwc1222/rigelledger/web"
)

type App struct {
	cfg    *Config
	router *routes.Router
	db     *Postgres
	logger *slog.Logger
	wg     sync.WaitGroup
}

const (
	defaultIdleTimeout    = time.Minute
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = 10 * time.Second
	defaultShutdownPeriod = 30 * time.Second
)

func NewApp(cfg *Config, logger *slog.Logger) (*App, error) {

	logger.Info("Initializing router")
	isLocalhost := cfg.AppURL == "localhost"
	logger.Info("Creating router access logger", "isLocalhost", isLocalhost)
	logFormat := httplog.SchemaECS.Concise(isLocalhost)
	accessLogger := NewLogger(LoggerConfig{
		AppName:    cfg.AppName,
		AppVersion: cfg.AppVersion,
		AppEnv:     cfg.AppEnv,
		LogLevel:   cfg.GetLogLevel(),
		Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:       cfg.GetLogLevel(),
			ReplaceAttr: logFormat.ReplaceAttr,
		}),
	})
	te := response.NewTemplateEngine(cfg.AppVersion, web.TemplateFiles)
	jwt := auth.New(cfg.AppURL, []string{cfg.AppName}, []byte(cfg.JWTSecret))
	router := routes.NewRouter(&routes.RouterConfig{
		AppURL:       cfg.AppURL,
		AppPort:      cfg.AppPort,
		AccessLogger: accessLogger,
		LogLevel:     cfg.GetLogLevel(),
	}, te, jwt, logger)

	db, err := NewPostgres(cfg, logger)
	if err != nil {
		logger.Error("Failed to connect to postgres", "error", err)
		return nil, err
	}

	return &App{
		cfg:    cfg,
		router: router,
		db:     db,
		logger: logger,
	}, nil
}

func (app *App) Serve() error {
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.cfg.AppURL, app.cfg.AppPort),
		Handler:      app.router.Handler,
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), app.cfg.GetLogLevel()),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}

	shutdownErrorChan := make(chan error)

	go func() {
		quitChan := make(chan os.Signal, 1)
		signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
		<-quitChan

		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownPeriod)
		defer cancel()

		// Close the database connection pool before shutting down the server
		app.logger.Info("Closing database connection pool")
		app.db.Close()

		shutdownErrorChan <- server.Shutdown(ctx)
	}()

	app.logger.Info("Starting server", "addr", server.Addr)

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	if err := <-shutdownErrorChan; err != nil {
		return err
	}

	app.logger.Info("Stopped server", slog.Group("server", "addr", server.Addr))

	app.wg.Wait()
	return nil
}
