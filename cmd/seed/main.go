package main

import (
	"context"
	"datastar-go/database"
	"datastar-go/database/sqlc"
	"log/slog"
	"os"
)

func main() {
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	q := sqlc.New(db)

	for i := range 3 {
		principal, err := q.CreatePrincipal(ctx, sqlc.PrincipalKindUser)
		if err != nil {
			slog.Error("create principal", "error", err)
			os.Exit(1)
		}
		slog.Info("principal created", "sno", i, "id", principal.ID, "pid", principal.Pid, "created_at", principal.CreatedAt.Time)
	}
}
