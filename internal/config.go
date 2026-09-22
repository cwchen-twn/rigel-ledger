package internal

import (
	"bufio"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	AppName    string `env:"APP_NAME" envDefault:"rigel-ledger"`
	AppVersion string `env:"APP_VERSION"`
	AppEnv     string `env:"APP_ENV" envDefault:"production"`
	AppURL     string `env:"APP_URL"`
	AppPort    int    `env:"APP_PORT" envDefault:"8080"`
	LogLevel   string `env:"LOG_LEVEL" envDefault:"info"`

	// SessionTTL is the sliding lifetime of a login: every request inside it
	// pushes the expiry forward again.
	SessionTTL time.Duration `env:"SESSION_TTL" envDefault:"720h"`

	// DatabaseURL, when set, wins over the PG_* parts. The hcloud chart sets
	// it from a SOPS secret; local development usually uses the parts.
	DatabaseURL string `env:"DATABASE_URL"`
	PgHost      string `env:"PG_HOST" envDefault:"localhost"`
	PgPort      int    `env:"PG_PORT" envDefault:"5432"`
	PgUser      string `env:"PG_USER"`
	PgPassword  string `env:"PG_PASS"`
	PgDbname    string `env:"APP_DBNAME"`
	PgMaxConns  int32  `env:"PG_MAX_CONNS" envDefault:"10"`
}

// DSN returns the postgres:// URL to connect to.
func (c *Config) DSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.PgUser, c.PgPassword),
		Host:     net.JoinHostPort(c.PgHost, strconv.Itoa(c.PgPort)),
		Path:     "/" + c.PgDbname,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

// IsDevelopment relaxes cookie security (no Secure flag) for plain-HTTP localhost.
func (c *Config) IsDevelopment() bool {
	return c.AppEnv == "development"
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

// loadDotEnv reads a .env file and sets each KEY=VALUE pair that is not already in the environment.
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

		// The real environment wins: .env only fills in what is unset, so a
		// stray .env can never override what a container or shell set.
		if _, exists := os.LookupEnv(key); key != "" && !exists {
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
