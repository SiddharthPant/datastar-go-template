package auth

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func SetupRoutes(ctx context.Context, router chi.Router, db *pgxpool.Pool) error {
	service := NewService(db)
	handler := NewHandler(service)
	router.Get("/auth/login", handler.LoginPage)
	router.Post("/auth/login", handler.Login)
	return nil
}
