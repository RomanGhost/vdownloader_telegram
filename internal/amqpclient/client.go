// Package amqpclient provides an RPC client for the vdownloader worker queues.
//
// Flow:
//   - GetFormats  → video.get_formats  (sync RPC)
//   - Download    → video.download     (sync RPC, returns job_id)
//   - ConsumeCompleted → video.completed (async events)
package amqpclient

import (
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client holds two AMQP channels:
//   - pubCh for publishing RPC requests
//   - subCh for consuming the exclusive reply queue and the completed queue
type Client struct {
	conn    *amqp.Connection
	pubCh   *amqp.Channel
	pubMu   sync.Mutex
	subCh   *amqp.Channel
	replyQ  string
	pending map[string]chan []byte
	mu      sync.Mutex
	log     *slog.Logger
}

// New dials RabbitMQ, declares all required queues, and sets up an exclusive
// reply queue for RPC. Safe to call once; the returned Client is goroutine-safe.
func New(amqpURL string, log *slog.Logger) (*Client, error) {
	log.Info("connecting to rabbitmq", "url", amqpURL)
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}

	pubCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open pub channel: %w", err)
	}

	subCh, err := conn.Channel()
	if err != nil {
		pubCh.Close()
		conn.Close()
		return nil, fmt.Errorf("open sub channel: %w", err)
	}

	// Ensure all worker queues exist before publishing.
	for _, name := range []string{queueGetFormats, queueDownload, queueCompleted} {
		if _, err := pubCh.QueueDeclare(name, true, false, false, false, nil); err != nil {
			subCh.Close()
			pubCh.Close()
			conn.Close()
			return nil, fmt.Errorf("declare queue %q: %w", name, err)
		}
	}

	// Exclusive, auto-delete reply queue for RPC callbacks.
	rq, err := subCh.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		subCh.Close()
		pubCh.Close()
		conn.Close()
		return nil, fmt.Errorf("declare reply queue: %w", err)
	}

	c := &Client{
		conn:    conn,
		pubCh:   pubCh,
		subCh:   subCh,
		replyQ:  rq.Name,
		pending: make(map[string]chan []byte),
		log:     log,
	}

	replies, err := subCh.Consume(rq.Name, "", true, true, false, false, nil)
	if err != nil {
		subCh.Close()
		pubCh.Close()
		conn.Close()
		return nil, fmt.Errorf("consume reply queue: %w", err)
	}
	go c.dispatchReplies(replies)

	log.Info("rabbitmq connected", "reply_queue", rq.Name)
	return c, nil
}

// Close shuts down all AMQP resources.
func (c *Client) Close() {
	c.log.Info("closing rabbitmq connection")
	c.subCh.Close()
	c.pubCh.Close()
	c.conn.Close()
}

