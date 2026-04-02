package amqpclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// ConsumeCompleted returns a channel that receives CompletedEvents as the
// worker finishes (or fails) each download job. The channel is closed when ctx
// is cancelled or the underlying AMQP delivery channel closes.
func (c *Client) ConsumeCompleted(ctx context.Context) (<-chan CompletedEvent, error) {
	deliveries, err := c.subCh.Consume(queueCompleted, "", true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("consume %s: %w", queueCompleted, err)
	}

	events := make(chan CompletedEvent, 16)
	go func() {
		defer close(events)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-deliveries:
				if !ok {
					return
				}
				var ev CompletedEvent
				if err := json.Unmarshal(msg.Body, &ev); err != nil {
					c.log.Error("decode completed event", "err", err)
					continue
				}
				c.log.Info("completed event received", "job_id", ev.JobID, "status", ev.Status)
				select {
				case events <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return events, nil
}
