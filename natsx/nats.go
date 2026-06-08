package natsx

import (
	"context"
	"datastar-go/config"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Client struct {
	Conn      *nats.Conn
	JetStream jetstream.JetStream
}

func New(ctx context.Context) (*Client, error) {
	opts := []nats.Option{
		nats.Name(config.Env.NATSName),
		nats.Timeout(config.Env.NATSConnectTimeout),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			slog.Warn("nats disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Info("nats reconnected", "url", nc.ConnectedUrlRedacted())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			slog.Info("nats connection closed")
		}),
	}

	nc, err := nats.Connect(config.Env.NATSURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	select {
	case <-ctx.Done():
		nc.Close()
		return nil, ctx.Err()
	default:
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create jetstream client: %w", err)
	}

	return &Client{
		Conn:      nc,
		JetStream: js,
	}, nil
}

func (c *Client) Close() {
	if c == nil || c.Conn == nil {
		return
	}

	if err := c.Conn.Drain(); err != nil {
		slog.Warn("failed to drain nats connection", "error", err)
		c.Conn.Close()
	}
}
