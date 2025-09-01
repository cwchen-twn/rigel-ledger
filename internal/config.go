package internal

import (
	"log/slog"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	AppName    string `mapstructure:"APP_NAME"`
	AppVersion string `mapstructure:"APP_VERSION"`
	AppEnv     string `mapstructure:"APP_ENV"`
	AppURL     string `mapstructure:"APP_URL"`
	AppPort    int    `mapstructure:"APP_PORT"`
	LogLevel   string `mapstructure:"LOG_LEVEL"`
	JWTSecret  string `mapstructure:"JWT_SECRET"`

	PgHost            string        `mapstructure:"PG_HOST"`
	PgPort            int           `mapstructure:"PG_PORT"`
	PgUser            string        `mapstructure:"PG_USER"`
	PgPassword        string        `mapstructure:"PG_PASS"`
	PgDbname          string        `mapstructure:"APP_DBNAME"`
	PgMaxOpenConns    int           `mapstructure:"PG_MAX_OPEN_CONN"`
	PgMaxIdleConns    int           `mapstructure:"PG_MAX_IDLE_CONNS"`
	PgMaxConnLifetime time.Duration `mapstructure:"PG_MAX_CONN_LIFETIME"`
	PgMaxConnIdleTime time.Duration `mapstructure:"PG_MAX_CONN_IDLE_TIME"`

	// PgMinConns        int           `mapstructure:"PG_MIN_CONN"`
	// PgMaxConnLifetimeJitter string        `mapstructure:"PG_MAX_CONN_LIFE_JITTER"`
	// PgHealthCheckPeriod     string        `mapstructure:"PG_HEALTH_CHECK_PERIOD"`
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

func (c *Config) IsLocalhost() bool {
	return c.AppURL == "localhost"
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
