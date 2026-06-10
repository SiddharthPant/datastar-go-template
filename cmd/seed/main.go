package main

import (
	"context"
	"datastar-go/database"
	"datastar-go/database/sqlc"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := seed(ctx, db); err != nil {
		slog.Error("seed", "error", err)
		os.Exit(1)
	}
	slog.Info("seeding is complete")
}

func seed(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	seedUsers := []struct {
		PrincipalID uuid.UUID
		Email       string
		Name        string
		IsStaff     bool
	}{
		{uuid.MustParse("019eb1d2-c71c-7c59-8ff9-ba86da8fdfef"), "admin@example.com", "Admin User", true},
		{uuid.MustParse("019eb1d2-c721-7bc6-babd-b361feb4074f"), "org_owner@example.com", "Owner Org", false},
		{uuid.MustParse("019eb1d2-c723-760d-9f9b-08bb65de6d77"), "org_admin@example.com", "Org Admin", false},
		{uuid.MustParse("019eb1d2-c724-7824-ba91-6a87af2d7254"), "team_admin@example.com", "Team Admin", false},
		{uuid.MustParse("019eb1d2-c725-75f5-acbc-7b659b3e390c"), "team_member@example.com", "Team Member", false},
	}

	q := sqlc.New(tx)

	for _, u := range seedUsers {
		principal, err := q.CreatePrincipal(ctx, sqlc.CreatePrincipalParams{
			ID:   u.PrincipalID,
			Kind: sqlc.PrincipalKindUser,
		})
		if err != nil {
			return fmt.Errorf("create principal %s: %w", u.Email, err)
		}
		slog.Info("principal created", "id", principal.ID, "pid", principal.Pid)

		user, err := q.UpsertUserForSeed(ctx, sqlc.UpsertUserForSeedParams{
			PrincipalID: u.PrincipalID,
			Email:       u.Email,
			Name:        u.Name,
			IsStaff:     u.IsStaff,
		})
		if err != nil {
			return fmt.Errorf("upsert user %s: %w", u.Email, err)
		}
		slog.Info("seeded user", "email", user.Email, "id", user.ID, "name", user.Email, "isStaff", user.IsStaff)
	}

	return tx.Commit(ctx)
}
