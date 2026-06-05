package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"datastar-go/config"
	"datastar-go/database"
	"datastar-go/natsx"
	"datastar-go/router"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		slog.Error("server failure", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: config.Global.LogLevel,
	}))
	slog.SetDefault(logger)

	db, err := database.New(ctx)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	natsClient, err := natsx.New(ctx)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer natsClient.Close()

	if err := natsClient.EnsureStreams(ctx); err != nil {
		return fmt.Errorf("ensure nats streams: %w", err)
	}

	r := chi.NewMux()
	r.Use(
		httplog.RequestLogger(logger, nil),
		middleware.Recoverer,
	)

	eg, egctx := errgroup.WithContext(ctx)

	if err := router.SetupRoutes(egctx, r, db, natsClient); err != nil {
		return fmt.Errorf("error setting up routes: %w", err)
	}

	addr := fmt.Sprintf("%s:%s", config.Global.Host, config.Global.Port)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ErrorLog: slog.NewLogLogger(
			slog.Default().Handler(),
			slog.LevelError,
		),
	}

	eg.Go(func() error {
		slog.Info("server started", "addr", srv.Addr)
		err := srv.ListenAndServe()

		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("server error: %w", err)
	})

	eg.Go(func() error {
		<-egctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}

		return nil
	})

	return eg.Wait()
}
