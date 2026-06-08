package router

import (
	"context"
	"datastar-go/config"
	"datastar-go/natsx"
	"datastar-go/web/resources"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	indexFeature "datastar-go/features/index"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starfederation/datastar-go/datastar"
)

func SetupRoutes(ctx context.Context, router chi.Router, db *pgxpool.Pool, natsClient *natsx.Client) (err error) {
	if config.Env.AppEnv == config.Local {
		setupReload(router)
	}
	router.Handle("/static/*", resources.Handler())

	if err := errors.Join(
		indexFeature.SetupRoutes(ctx, router, db, natsClient),
	); err != nil {
		return fmt.Errorf("error setting up routes: %w", err)
	}

	return nil
}

func setupReload(router chi.Router) {
	reloadChan := make(chan struct{}, 1)
	var hotReloadOnce sync.Once

	router.Get("/reload", func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r)
		reload := func() {
			err := sse.ExecuteScript("window.location.reload()")
			if err != nil {
				slog.Error("failed to reload page", "error", err)
			}
		}
		hotReloadOnce.Do(reload)
		select {
		case <-reloadChan:
			reload()
		case <-r.Context().Done():
		}
	})

	router.Get("/hotreload", func(w http.ResponseWriter, r *http.Request) {
		select {
		case reloadChan <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			slog.Error("failed to write response", "error", err)
			return
		}
	})
}
