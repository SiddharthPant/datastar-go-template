package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"datastar-go/config"
	"datastar-go/database"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const migrationsDir = "migrations"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: migrate [up|down|status]")
	}

	db, err := sql.Open("pgx", config.Env.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			slog.Error("close database", "error", err)
		}
	}(db)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	goose.SetBaseFS(database.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	switch command {
	case "up":
		return goose.UpContext(ctx, db, migrationsDir)
	case "down":
		return goose.DownContext(ctx, db, migrationsDir)
	case "status":
		return goose.StatusContext(ctx, db, migrationsDir)
	default:
		return fmt.Errorf("unknown migration command %q", command)
	}
}
