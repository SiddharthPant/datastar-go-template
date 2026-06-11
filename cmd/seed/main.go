package main

import (
	"context"
	"datastar-go/config"
	"datastar-go/database"
	"datastar-go/database/sqlc"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type teamSeed struct {
	ID   uuid.UUID
	Name string
	Slug string
}

type userSeed struct {
	ID    uuid.UUID
	Email string
	Name  string
	Role  sqlc.UserRole
}

type teamMembershipSeed struct {
	ID     uuid.UUID
	TeamID uuid.UUID
	UserID uuid.UUID
}

var seedTeams = []teamSeed{
	{ID: uuid.MustParse("019eb5e8-adaf-7864-82ae-7555a36ee216"), Name: "Platform", Slug: "platform"},
	{ID: uuid.MustParse("019eb5e8-f0a4-7dcd-bf19-e4b3c48302d9"), Name: "Operations", Slug: "operations"},
}

var seedUsers = []userSeed{
	{ID: uuid.MustParse("019eb1d2-c71c-7c59-8ff9-ba86da8fdfef"), Email: "owner@example.com", Name: "Owner User", Role: sqlc.UserRoleOwner},
	{ID: uuid.MustParse("019eb1d2-c721-7bc6-babd-b361feb4074f"), Email: "admin@example.com", Name: "Admin User", Role: sqlc.UserRoleAdmin},
	{ID: uuid.MustParse("019eb1d2-c723-760d-9f9b-08bb65de6d77"), Email: "member@example.com", Name: "Member User", Role: sqlc.UserRoleMember},
}

var seedTeamMemberships = []teamMembershipSeed{
	{
		ID:     uuid.MustParse("019eb5e9-3a42-7a87-b077-0294e1fcb30d"),
		TeamID: uuid.MustParse("019eb5e8-f0a4-7dcd-bf19-e4b3c48302d9"),
		UserID: uuid.MustParse("019eb1d2-c721-7bc6-babd-b361feb4074f"),
	},
	{
		ID:     uuid.MustParse("019eb5e9-9076-77b6-9bf6-e122e7830e21"),
		TeamID: uuid.MustParse("019eb5e8-f0a4-7dcd-bf19-e4b3c48302d9"),
		UserID: uuid.MustParse("019eb1d2-c723-760d-9f9b-08bb65de6d77"),
	},
}

func main() {
	if config.Env.AppEnv == config.Prod {
		slog.Error("refusing to seed in prod")
		os.Exit(1)
	}
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

	q := sqlc.New(tx)

	for i, u := range seedUsers {
		user, err := q.UpsertUserForSeed(ctx, sqlc.UpsertUserForSeedParams{
			ID:           u.ID,
			Email:        u.Email,
			Name:         u.Name,
			Role:         u.Role,
			PasswordHash: nil,
		})
		if err != nil {
			return fmt.Errorf("upsert user %s: %w", u.Email, err)
		}
		slog.Info("seeded user", "row", i+1, "email", user.Email, "id", user.ID, "name", user.Name, "role", user.Role)
	}

	for i, t := range seedTeams {
		team, err := q.UpsertTeamForSeed(ctx, sqlc.UpsertTeamForSeedParams{
			ID:   t.ID,
			Name: t.Name,
			Slug: t.Slug,
		})
		if err != nil {
			return fmt.Errorf("upsert team %s: %w", t.Slug, err)
		}
		slog.Info("seeded team", "row", i+1, "slug", team.Slug, "id", team.ID)
	}

	for i, m := range seedTeamMemberships {
		membership, err := q.UpsertTeamMembershipForSeed(ctx, sqlc.UpsertTeamMembershipForSeedParams{
			ID:     m.ID,
			TeamID: m.TeamID,
			UserID: m.UserID,
		})
		if err != nil {
			return fmt.Errorf("upsert team membership %s: %w", m.ID, err)
		}
		slog.Info("seeded team membership", "row", i+1, "team_id", membership.TeamID, "user_id", membership.UserID)
	}

	return tx.Commit(ctx)
}
