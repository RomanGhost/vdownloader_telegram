package amqpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// GetFormats performs a synchronous RPC to fetch the video title and formats.
func (c *Client) GetFormats(ctx context.Context, url string) (*GetFormatsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	c.log.Debug("rpc get_formats", "url", url)
	raw, err := c.call(ctx, queueGetFormats, GetFormatsRequest{URL: url})
	if err != nil {
		c.log.Error("rpc get_formats failed", "url", url, "err", err)
		return nil, err
	}
	var resp GetFormatsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != "" {
		c.log.Warn("rpc get_formats error from worker", "url", url, "err", resp.Error)
		return nil, fmt.Errorf("%s", resp.Error)
	}
	c.log.Info("rpc get_formats ok", "url", url, "title", resp.Title, "formats", len(resp.Formats))
	return &resp, nil
}

// Download performs a synchronous RPC to enqueue a download job and returns the job ID.
func (c *Client) Download(ctx context.Context, req DownloadRequest) (*DownloadResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	c.log.Debug("rpc download", "url", req.URL, "format", req.FormatArg)
	raw, err := c.call(ctx, queueDownload, req)
	if err != nil {
		c.log.Error("rpc download failed", "url", req.URL, "err", err)
		return nil, err
	}
	var resp DownloadResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != "" {
		c.log.Warn("rpc download error from worker", "url", req.URL, "err", resp.Error)
		return nil, fmt.Errorf("%s", resp.Error)
	}
	c.log.Info("rpc download queued", "url", req.URL, "job_id", resp.JobID)
	return &resp, nil
}

// call publishes a JSON request to the given queue with a unique correlation ID
// and waits for the reply on the exclusive reply queue.
func (c *Client) call(ctx context.Context, queue string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	corrID := uuid.NewString()
	replyCh := make(chan []byte, 1)

	c.mu.Lock()
	c.pending[corrID] = replyCh
	c.mu.Unlock()

	c.pubMu.Lock()
	err = c.pubCh.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:   "application/json",
		CorrelationId: corrID,
		ReplyTo:       c.replyQ,
		Body:          data,
	})
	c.pubMu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.pending, corrID)
		c.mu.Unlock()
		return nil, fmt.Errorf("publish: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, corrID)
		c.mu.Unlock()
		return nil, ctx.Err()
	case raw := <-replyCh:
		return raw, nil
	}
}

// dispatchReplies routes incoming RPC replies to the waiting call() goroutines.
func (c *Client) dispatchReplies(msgs <-chan amqp.Delivery) {
	for msg := range msgs {
		c.mu.Lock()
		ch, ok := c.pending[msg.CorrelationId]
		if ok {
			delete(c.pending, msg.CorrelationId)
		}
		c.mu.Unlock()
		if ok {
			ch <- msg.Body
		}
	}
}
