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
		Addr:         fmt.Sprintf("%s:%d", app.cfg.AppUrl, app.cfg.AppPort),
		Handler:      app.router,
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}

	shutdownErrorChan := make(chan error)

	go func() {
		quitChan := make(chan os.Signal, 1)
		signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quitChan
		app.logger.Info("Shutting down server", "signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownPeriod)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			shutdownErrorChan <- err
		}

		app.logger.Info("Completing background tasks", "error", <-shutdownErrorChan)
		close(shutdownErrorChan)
	}()

	app.logger.Info("Starting server", "addr", server.Addr)

	err := server.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err = <-shutdownErrorChan
	if err != nil {
		return err
	}

	app.logger.Info("stopped server", slog.Group("server", "addr", server.Addr))

	app.wg.Wait()
	return nil
}

func main() {
	logger := internal.NewLogger()

	cfg, err := internal.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("Initializing database connection pool", "host", cfg.PgHost, "port", cfg.PgPort, "dbname", cfg.PgDbname)
	db, err := internal.NewPgPool(cfg)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("Initializing router")
	router := internal.NewRouter(cfg.AppUrl, cfg.AppPort)

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
