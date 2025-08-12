package main

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

	_ "github.com/cwc1222/rigelledger/docs"
	"github.com/cwc1222/rigelledger/internal"
	"github.com/go-chi/httplog/v3"
)

const (
	defaultIdleTimeout    = time.Minute
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = 10 * time.Second
	defaultShutdownPeriod = 30 * time.Second
)

type application struct {
	cfg    *internal.Config
	router http.Handler
	logger *slog.Logger
	wg     sync.WaitGroup
}

func (app *application) serve() error {
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.cfg.AppURL, app.cfg.AppPort),
		Handler:      app.router,
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

func main() {

	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Println("Failed to load config", err)
		os.Exit(1)
	}

	logger := internal.NewLogger(internal.LoggerConfig{
		AppName:    cfg.AppName,
		AppVersion: cfg.AppVersion,
		AppEnv:     cfg.AppEnv,
		LogLevel:   cfg.GetLogLevel(),
		Handler:    slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.GetLogLevel()}),
	})

	logger.Info("Initializing database connection pool", "host", cfg.PgHost, "port", cfg.PgPort, "dbname", cfg.PgDbname)
	db, err := internal.NewPgPool(cfg)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("Initializing router")
	isLocalhost := cfg.AppURL == "localhost"
	logger.Info("Creating router access logger", "isLocalhost", isLocalhost)
	logFormat := httplog.SchemaECS.Concise(isLocalhost)
	accessLogger := internal.NewLogger(internal.LoggerConfig{
		AppName:    cfg.AppName,
		AppVersion: cfg.AppVersion,
		AppEnv:     cfg.AppEnv,
		LogLevel:   cfg.GetLogLevel(),
		Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:       cfg.GetLogLevel(),
			ReplaceAttr: logFormat.ReplaceAttr,
		}),
	})
	router := internal.NewRouter(internal.RouterConfig{
		AppURL:       cfg.AppURL,
		AppPort:      cfg.AppPort,
		AccessLogger: accessLogger,
		LogLevel:     cfg.GetLogLevel(),
	})

	logger.Info("Initializing application")
	app := &application{
		cfg:    cfg,
		logger: logger,
		router: router,
	}

	if err := app.serve(); err != nil {
		logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
