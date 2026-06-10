package config

import (
	"log/slog"
	"sync"
	"time"
)

type Config struct {
	AppEnv       AppEnv
	Host         string
	Port         string
	LogLevel     slog.Level
	AppBaseURL   string
	SeedPassword string

	DatabaseURL      string
	DBMaxConns       int
	DBMinConns       int
	DBConnectTimeout time.Duration
	DBIdleTimeout    time.Duration

	NATSURL            string
	NATSName           string
	NATSConnectTimeout time.Duration

	SMTPAddr string
	SMTPFrom string
}

var (
	Env  *Config
	once sync.Once
)

func init() {
	once.Do(func() {
		Env = load()
	})
}

func load() *Config {
	config := &Config{
		AppEnv:       GetAppEnv("APP_ENV"),
		Host:         GetString("HOST"),
		Port:         GetString("PORT"),
		LogLevel:     GetSlogLevel("LOG_LEVEL"),
		AppBaseURL:   GetString("APP_BASE_URL"),
		SeedPassword: GetString("SEED_PASSWORD"),

		DatabaseURL:        GetString("DATABASE_URL"),
		DBMaxConns:         GetInt("DATABASE_MAX_CONNECTIONS"),
		DBMinConns:         GetInt("DATABASE_MIN_CONNECTIONS"),
		DBConnectTimeout:   GetDuration("DATABASE_CONNECT_TIMEOUT"),
		DBIdleTimeout:      GetDuration("DATABASE_IDLE_TIMEOUT"),
		NATSURL:            GetString("NATS_URL"),
		NATSName:           GetString("NATS_NAME"),
		NATSConnectTimeout: GetDuration("NATS_CONNECT_TIMEOUT"),

		SMTPAddr: GetString("SMTP_ADDR"),
		SMTPFrom: GetString("SMTP_FROM"),
	}
	return config
}
