package internal

import (
	"github.com/spf13/viper"
)

type Config struct {
	AppUrl  string `mapstructure:"APP_URL"`
	AppPort int    `mapstructure:"APP_PORT"`

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
