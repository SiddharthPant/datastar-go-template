package config

import (
	"log/slog"
	"os"
	"sync"
	"time"
)

type Config struct {
	AppEnv        Environment
	Host          string
	Port          string
	LogLevel      slog.Level
	SessionSecret string

	DatabaseURL      string
	DBMaxConns       int
	DBMinConns       int
	DBConnectTimeout time.Duration
	DBIdleTimeout    time.Duration

	NATSURL            string
	NATSName           string
	NATSConnectTimeout time.Duration
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
	env := &EnvLoader{}

	config := &Config{
		AppEnv:        env.appEnv("APP_ENV", "", true),
		Host:          env.string("HOST", "", true),
		Port:          env.string("PORT", "", true),
		LogLevel:      env.slogLevel("LOG_LEVEL", slog.LevelInfo, false),
		SessionSecret: env.string("SESSION_SECRET", "", true),

		DatabaseURL:      env.string("DATABASE_URL", "", true),
		DBMaxConns:       env.int("DATABASE_MAX_CONNECTIONS", 10, false),
		DBMinConns:       env.int("DATABASE_MIN_CONNECTIONS", 0, false),
		DBConnectTimeout: env.duration("DATABASE_CONNECT_TIMEOUT", 5*time.Second, false),
		DBIdleTimeout:    env.duration("DATABASE_IDLE_TIMEOUT", 30*time.Second, false),

		NATSURL:            env.string("NATS_URL", "", true),
		NATSName:           env.string("NATS_NAME", "datastar-go", true),
		NATSConnectTimeout: env.duration("NATS_CONNECT_TIMEOUT", 5*time.Second, true),
	}
	if err := env.err(); err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	return config
}
