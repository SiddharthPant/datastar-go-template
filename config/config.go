package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

type Environment string

const (
	Dev  Environment = "dev"
	Prod Environment = "prod"
)

type Config struct {
	Environment   Environment
	Host          string
	Port          string
	LogLevel      slog.Level
	SessionSecret string

	DatabaseURL      string
	DbMaxConns       int
	DbMinConns       int
	DbConnectTimeout time.Duration
	DbIdleTimeout    time.Duration

	NatsUrl            string
	NatsName           string
	NatsConnectTimeout time.Duration
}

var (
	Global *Config
	once   sync.Once
)

func init() {
	once.Do(func() {
		Global = Load()
	})
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}

	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(strings.ReplaceAll(v, " ", "")); err == nil {
			return d
		}
	}

	return fallback
}

func loadBase() *Config {
	err := godotenv.Load()
	if err != nil {
		slog.Error("failed to load .env file", "error", err)
		return nil
	}

	return &Config{
		Host: getEnv("HOST", "0.0.0.0"),
		Port: getEnv("PORT", "8080"),
		LogLevel: func() slog.Level {
			switch os.Getenv("LOG_LEVEL") {
			case "DEBUG":
				return slog.LevelDebug
			case "INFO":
				return slog.LevelInfo
			case "WARN":
				return slog.LevelWarn
			case "ERROR":
				return slog.LevelError
			default:
				return slog.LevelInfo
			}
		}(),
		SessionSecret: getEnv("SESSION_SECRET", "session-secret"),

		DatabaseURL:      getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/app_db"),
		DbMaxConns:       getEnvInt("DATABASE_MAX_CONNECTIONS", 10),
		DbMinConns:       getEnvInt("DATABASE_MIN_CONNECTIONS", 0),
		DbConnectTimeout: getEnvDuration("DATABASE_CONNECT_TIMEOUT", 5*time.Second),
		DbIdleTimeout:    getEnvDuration("DATABASE_IDLE_TIMEOUT", 30*time.Second),

		NatsUrl:            getEnv("NATS_URL", "nats://localhost:4222"),
		NatsName:           getEnv("NATS_NAME", "datastar-go"),
		NatsConnectTimeout: getEnvDuration("NATS_CONNECT_TIMEOUT", 5*time.Second),
	}
}
