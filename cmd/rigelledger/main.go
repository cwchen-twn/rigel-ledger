package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/cwc1222/rigelledger/internal"
)

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

	logger.Info("Initializing application")
	app, err := internal.NewApp(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize application", "error", err)
		os.Exit(1)
	}

	if err := app.Serve(); err != nil {
		logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
