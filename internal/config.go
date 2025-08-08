package internal

import (
	"log/slog"

	"github.com/spf13/viper"
)

type Config struct {
	AppName    string `mapstructure:"APP_NAME"`
	AppVersion string `mapstructure:"APP_VERSION"`
	AppEnv     string `mapstructure:"APP_ENV"`
	AppUrl     string `mapstructure:"APP_URL"`
	AppPort    int    `mapstructure:"APP_PORT"`
	LogLevel   string `mapstructure:"LOG_LEVEL"`

	PgHost                  string `mapstructure:"PG_HOST"`
	PgPort                  int    `mapstructure:"PG_PORT"`
	PgUser                  string `mapstructure:"PG_USER"`
	PgPassword              string `mapstructure:"PG_PASS"`
	PgDbname                string `mapstructure:"APP_DBNAME"`
	PgMaxConns              int    `mapstructure:"PG_MAX_CONN"`
	PgMinConns              int    `mapstructure:"PG_MIN_CONN"`
	PgMaxConnLifetime       string `mapstructure:"PG_MAX_CONN_LIFE"`
	PgMaxConnIdleTime       string `mapstructure:"PG_CONN_IDLE_TIME"`
	PgMaxConnLifetimeJitter string `mapstructure:"PG_MAX_CONN_LIFE_JITTER"`
	PgHealthCheckPeriod     string `mapstructure:"PG_HEALTH_CHECK_PERIOD"`
}

// GetLogLevel returns the slog.Level based on the configured LogLevel string
func (c *Config) GetLogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo // default to info level
	}
}

func LoadConfig() (*Config, error) {
	var cfg Config

	viper.SetConfigFile(".env")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
