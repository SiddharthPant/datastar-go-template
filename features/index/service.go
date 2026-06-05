package index

import (
	"context"
	"datastar-go/database/sqlc"
	"datastar-go/natsx"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const demoPingSubject = "demo.ping"

type Service struct {
	queries    *sqlc.Queries
	natsClient *natsx.Client
}

type demoJob struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Message   string    `json:"message"`
}

func NewService(db *pgxpool.Pool, natsClient *natsx.Client) *Service {
	return &Service{
		queries:    sqlc.New(db),
		natsClient: natsClient,
	}
}

func (s *Service) Setup(ctx context.Context) error {
	_, err := s.natsClient.Conn.Subscribe(demoPingSubject, func(msg *nats.Msg) {
		if err := msg.Respond([]byte("pong")); err != nil {
			slog.Error("failed to respond to nats ping", "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe demo ping: %w", err)
	}

	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.natsClient.Conn.FlushWithContext(flushCtx); err != nil {
		return fmt.Errorf("flush nats subscriptions: %w", err)
	}

	return nil
}

func (s *Service) DbTime(ctx context.Context) (string, error) {
	dbTime, err := s.queries.HealthCheck(ctx)
	if err != nil {
		return "", fmt.Errorf("health query: %w", err)
	}

	return dbTime, nil
}

func (s *Service) PingNats(ctx context.Context) (string, error) {
	msg, err := s.natsClient.Conn.RequestWithContext(ctx, demoPingSubject, []byte("ping"))
	if err != nil {
		return "", fmt.Errorf("request nate ping: %w", err)
	}

	return string(msg.Data), nil
}

func (s *Service) DemoJobsCount(ctx context.Context) (uint64, error) {
	return s.natsClient.DemoJobsCount(ctx)
}

func (s *Service) PublishDemoJob(ctx context.Context) (uint64, error) {
	job := demoJob{
		ID:        uuid.NewString(),
		CreatedAt: time.Now().UTC(),
		Message:   "hello from datastar-go",
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return 0, fmt.Errorf("encode demo job: %w", err)
	}

	_, err = s.natsClient.Js.Publish(
		ctx,
		natsx.DemoJobsSubject,
		payload,
		jetstream.WithExpectStream(natsx.DemoJobsStream),
		jetstream.WithMsgID(job.ID),
	)
	if err != nil {
		return 0, fmt.Errorf("publish demo job: %w", err)
	}

	count, err := s.natsClient.DemoJobsCount(ctx)
	if err != nil {
		return 0, fmt.Errorf("get demo jobs count: %w", err)
	}

	return count, nil
}
