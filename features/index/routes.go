package index

import (
	"context"
	"datastar-go/natsx"
	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func SetupRoutes(ctx context.Context, router chi.Router, db *pgxpool.Pool, natsClient *natsx.Client) error {
	service := NewService(db, natsClient)
	if err := service.Setup(ctx); err != nil {
		return fmt.Errorf("setup index service: %w", err)
	}

	handler := NewHandler(service)

	router.Get("/", handler.Index)
	router.Get("/increment", handler.Increment)
	router.Get("/nats/ping", handler.PingNATS)
	router.Post("/jobs/demo", handler.PublishDemoJob)

	return nil
}
