package natsx

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	DemoJobsStream  = "DEMO_JOBS"
	DemoJobsSubject = "jobs.demo"
)

func (c *Client) EnsureStreams(ctx context.Context) error {
	_, err := c.Js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      DemoJobsStream,
		Subjects:  []string{DemoJobsSubject},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		MaxAge:    24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("ensure demo jobs stream: %w", err)
	}

	return nil
}

func (c *Client) DemoJobsCount(ctx context.Context) (uint64, error) {
	stream, err := c.Js.Stream(ctx, DemoJobsStream)
	if err != nil {
		return 0, fmt.Errorf("get demo jobs stream: %w", err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return 0, fmt.Errorf("get demo jobs stream info: %w", err)
	}

	return info.State.Msgs, nil
}
