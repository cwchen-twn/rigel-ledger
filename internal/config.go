package internal

import (
	"bufio"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	AppName    string `env:"APP_NAME"`
	AppVersion string `env:"APP_VERSION"`
	AppEnv     string `env:"APP_ENV"`
	AppURL     string `env:"APP_URL"`
	AppPort    int    `env:"APP_PORT"`
	LogLevel   string `env:"LOG_LEVEL"`
	JWTSecret  string `env:"JWT_SECRET"`

	PgHost            string        `env:"PG_HOST"`
	PgPort            int           `env:"PG_PORT"`
	PgUser            string        `env:"PG_USER"`
	PgPassword        string        `env:"PG_PASS"`
	PgDbname          string        `env:"APP_DBNAME"`
	PgMaxOpenConns    int           `env:"PG_MAX_OPEN_CONN"`
	PgMaxIdleConns    int           `env:"PG_MAX_IDLE_CONNS"`
	PgMaxConnLifetime time.Duration `env:"PG_MAX_CONN_LIFETIME"`
	PgMaxConnIdleTime time.Duration `env:"PG_MAX_CONN_IDLE_TIME"`

	// PgMinConns        int           `env:"PG_MIN_CONN"`
	// PgMaxConnLifetimeJitter string        `env:"PG_MAX_CONN_LIFE_JITTER"`
	// PgHealthCheckPeriod     string        `env:"PG_HEALTH_CHECK_PERIOD"`
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

// loadDotEnv reads a .env file and sets each KEY=VALUE pair as an environment variable.
// It skips blank lines and lines starting with '#'. Inline comments and surrounding quotes are stripped.
func loadDotEnv(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env file is optional
		}
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip inline comment (# not inside quotes)
		if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}

		// Strip surrounding quotes (single or double)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if key != "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

func LoadConfig() (*Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return nil, err
	}

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
