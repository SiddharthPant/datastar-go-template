package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Environment string

const (
	Local  Environment = "local"
	Server Environment = "server"
)

type EnvLoader struct {
	errs []error
}

func (e *EnvLoader) err() error {
	return errors.Join(e.errs...)
}

func (e *EnvLoader) addErr(format string, args ...any) {
	e.errs = append(e.errs, fmt.Errorf(format, args...))
}

func (e *EnvLoader) string(key string, fallback string, required bool) string {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		return value
	}
	if required {
		e.addErr("required environment variable %s is not set", key)
	}
	slog.Warn("required environment variable is not set, using fallback", "key", key, "value", value, "fallback", fallback)
	return fallback
}

func (e *EnvLoader) int(key string, fallback int, required bool) int {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	if required {
		e.addErr("required environment variable %s is not set", key)
	}
	slog.Warn("required environment variable is not set, using fallback", "key", key, "value", value, "fallback", fallback)
	return fallback
}

func (e *EnvLoader) duration(key string, fallback time.Duration, required bool) time.Duration {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	if required {
		e.addErr("required environment variable %s is not set", key)
	}
	slog.Warn("required environment variable is not set, using fallback", "key", key, "value", value, "fallback", fallback)
	return fallback
}

func (e *EnvLoader) bool(key string, fallback bool, required bool) bool {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	if required {
		e.addErr("required environment variable %s is not set", key)
	}
	slog.Warn("required environment variable is not set, using fallback", "key", key, "value", value, "fallback", fallback)
	return fallback
}

func (e *EnvLoader) slogLevel(key string, fallback slog.Level, required bool) slog.Level {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(value)); err == nil {
			return level
		}
	}
	if required {
		e.addErr("required environment variable %s is not set", key)
	}
	slog.Warn("required environment variable is not set, using fallback", "key", key, "value", value, "fallback", fallback)
	return fallback
}

func (e *EnvLoader) appEnv(key string, fallback Environment, required bool) Environment {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		return Environment(value)
	}
	if required {
		e.addErr("required environment variable %s is not set", key)
	}
	slog.Warn("required environment variable is not set, using fallback", "key", key, "value", value, "fallback", fallback)
	return fallback
}
