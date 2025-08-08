package internal

import (
	"log/slog"
	"os"
)

type LoggerConfig struct {
	AppName    string
	AppVersion string
	AppEnv     string
	Handler    slog.Handler
	LogLevel   slog.Level
}

func NewLogger(config LoggerConfig) *slog.Logger {
	var logger *slog.Logger

	if config.Handler != nil {
		logger = slog.New(config.Handler)
	} else {
		// Use the configured log level for the default handler
		handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: config.LogLevel,
		})
		logger = slog.New(handler)
	}

	return logger.With(
		slog.String("app", config.AppName),
		slog.String("version", config.AppVersion),
		slog.String("env", config.AppEnv),
	)
}
