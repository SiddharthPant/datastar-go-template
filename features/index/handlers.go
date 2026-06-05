package index

import (
	"datastar-go/features/index/pages"
	"log/slog"
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

var count int

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	dbTime, err := h.service.DBTime(r.Context())
	if err != nil {
		slog.Error("health query failed", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	jobsCount, err := h.service.DemoJobsCount(r.Context())
	if err != nil {
		slog.Error("demo jobs count failed", "error", err)
		jobsCount = 0
	}

	if err := pages.IndexPage(count, dbTime, "ready", jobsCount).Render(r.Context(), w); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) Increment(w http.ResponseWriter, r *http.Request) {
	count++

	sse := datastar.NewSSE(w, r)
	if err := sse.PatchElementTempl(pages.Counter(count)); err != nil {
		slog.Error("failed to patch counter element", "error", err)
	}
}

func (h *Handler) PingNATS(w http.ResponseWriter, r *http.Request) {
	message, err := h.service.PingNATS(r.Context())
	if err != nil {
		slog.Error("nats ping failed", "error", err)
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	sse := datastar.NewSSE(w, r)
	if err := sse.PatchElementTempl(pages.NATSPing(message)); err != nil {
		slog.Error("failed to patch nats ping", "error", err)
	}
}

func (h *Handler) PublishDemoJob(w http.ResponseWriter, r *http.Request) {
	jobsCount, err := h.service.PublishDemoJob(r.Context())
	if err != nil {
		slog.Error("failed to publish demo job", "error", err)
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	sse := datastar.NewSSE(w, r)
	if err := sse.PatchElementTempl(pages.JobsCount(jobsCount)); err != nil {
		slog.Error("failed to patch jobs count", "error", err)
	}
}
