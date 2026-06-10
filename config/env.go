package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

type AppEnv string

const (
	Dev  AppEnv = "dev"
	Prod AppEnv = "prod"
)

func GetString(key string) string {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		return value
	}

	slog.Error("required env var is not set", "envVar", key, "value", value)
	panic(1)
}

func GetInt(key string) int {
	return parseValue(key, strconv.Atoi, "cannot parse required env var value to int")
}

func GetDuration(key string) time.Duration {
	return parseValue(key, time.ParseDuration, "cannot parse required env var value to duration")
}

func GetBool(key string) bool {
	return parseValue(key, strconv.ParseBool, "cannot parse required env var value to bool")
}

func parseValue[T any](key string, parser func(value string) (T, error), errorMessage string) T {
	value := GetString(key)
	parsedValue, err := parser(value)
	if err == nil {
		return parsedValue
	}

	slog.Error(errorMessage, "envVar", key, "value", value, "error", err.Error())
	panic(1)
}

func GetSlogLevel(key string) slog.Level {
	var level slog.Level
	return parseValue(key, func(rawValue string) (slog.Level, error) {
		err := level.UnmarshalText([]byte(rawValue))
		return level, err
	}, "cannot parse required env var value to slog.Level")
}

func GetAppEnv(key string) AppEnv {
	return parseValue(key, func(rawValue string) (AppEnv, error) {
		return AppEnv(rawValue), nil
	}, "cannot parse required env var value to AppEnv")
}
