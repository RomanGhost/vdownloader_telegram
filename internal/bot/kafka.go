package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	kafkago "github.com/segmentio/kafka-go"

	"tgbot/internal/workerclient"
)

// completedMessage mirrors the JSON value the worker publishes to the topic.
type completedMessage struct {
	FileID string `json:"file_id"`
}

// startKafkaConsumer reads completed job IDs from Kafka and dispatches file
// delivery. It blocks until ctx is cancelled.
func (b *Bot) startKafkaConsumer(ctx context.Context) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: b.cfg.KafkaBrokersList(),
		Topic:   b.cfg.KafkaTopic,
		GroupID: "vdownloader-telegram",
	})

	go func() {
		<-ctx.Done()
		if err := reader.Close(); err != nil {
			b.log.Error("kafka reader close", "err", err)
		}
	}()

	go func() {
		b.log.Info("kafka consumer listening", "brokers", b.cfg.KafkaBrokers, "topic", b.cfg.KafkaTopic)
		for {
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, kafkago.ErrGroupClosed) {
					return
				}
				b.log.Error("kafka read message", "err", err)
				continue
			}

			var event completedMessage
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				b.log.Warn("kafka: bad payload", "err", err)
				continue
			}

			b.log.Info("kafka message received", "file_id", event.FileID)
			// Use a fresh context so the upload is not cancelled by consumer shutdown.
			go b.processCompleted(context.Background(), event.FileID)
		}
	}()
}

func (b *Bot) processCompleted(ctx context.Context, fileID string) {
	b.mu.Lock()
	job, ok := b.jobs[fileID]
	if ok {
		delete(b.jobs, fileID)
	}
	b.mu.Unlock()

	if !ok {
		b.log.Warn("completed event for unknown job", "file_id", fileID)
		return
	}

	editText := func(text string) {
		if _, err := b.tg.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    job.ChatID,
			MessageID: job.MsgID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
		}); err != nil {
			b.log.Warn("edit status message", "err", err)
		}
	}

	result, err := b.worker.GetJob(ctx, fileID)
	if err != nil {
		b.log.Error("get job", "file_id", fileID, "err", err)
		editText(fmt.Sprintf("<b>%s</b>\n\nFailed to fetch job status: %s",
			escapeHTML(job.Title), escapeHTML(err.Error())))
		return
	}

	if result.Status != "ready" {
		editText(fmt.Sprintf("<b>%s</b>\n\nDownload failed: %s",
			escapeHTML(job.Title), escapeHTML(result.Error)))
		return
	}

	editText(fmt.Sprintf("<b>%s</b>\n\nDone! Sending file…", escapeHTML(job.Title)))

	if err := b.sendFile(ctx, job.ChatID, result.FileID, job.Title, job.AudioOnly); err != nil {
		b.log.Error("send file", "file_id", fileID, "err", err)
		editText(fmt.Sprintf("<b>%s</b>\n\nReady but failed to send: %s",
			escapeHTML(job.Title), escapeHTML(err.Error())))
		return
	}

	editText(fmt.Sprintf("<b>%s</b>\n\nDone! ✓", escapeHTML(job.Title)))
}

// publishJobRequest sends a download job request to the worker's job-requests topic.
func (b *Bot) publishJobRequest(ctx context.Context, req workerclient.DownloadRequest) error {
	value, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal job request: %w", err)
	}
	return b.jobsWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(req.FileID),
		Value: value,
	})
}

// logValue satisfies slog.LogValuer so Bot can be used in structured log lines.
var _ slog.LogValuer = (*Bot)(nil)

func (b *Bot) LogValue() slog.Value { return slog.StringValue("bot") }
