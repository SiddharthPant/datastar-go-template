package index

import (
	"datastar-go/database/sqlc"
	"datastar-go/features/index/pages"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starfederation/datastar-go/datastar"
)

var count int

func SetupRoutes(router chi.Router, db *pgxpool.Pool) error {
	queries := sqlc.New(db)

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		dbTime, err := queries.HealthCheck(r.Context())
		if err != nil {
			slog.Error("health query failed", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if err := pages.IndexPage(count, dbTime).Render(r.Context(), w); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})

	router.Get("/increment", func(w http.ResponseWriter, r *http.Request) {
		count++
		sse := datastar.NewSSE(w, r)
		err := sse.PatchElementTempl(pages.Counter(count))
		if err != nil {
			slog.Error("failed to patch counter element", "error", err)
			return
		}
	})

	return nil
}
