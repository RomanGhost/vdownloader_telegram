package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"tgbot/internal/workerclient"
)

// completedMessage mirrors the JSON body the worker publishes to "video.completed".
type completedMessage struct {
	FileID string `json:"file_id"`
}

// handleCompleted is the "video.completed" consumer callback: it parses the
// finished job's file_id and hands off to processCompleted on a fresh context
// so an in-flight upload isn't cancelled when the consumer shuts down.
func (b *Bot) handleCompleted(_ context.Context, body []byte) {
	var event completedMessage
	if err := json.Unmarshal(body, &event); err != nil {
		b.log.Warn("mq: bad completion payload", "err", err)
		return
	}
	b.log.Info("mq: completion received", "file_id", event.FileID)
	go b.processCompleted(context.Background(), event.FileID)
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

// publishJobRequest publishes req to the "video.jobs" queue for the worker to pick up.
func (b *Bot) publishJobRequest(ctx context.Context, req workerclient.DownloadRequest) error {
	value, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal job request: %w", err)
	}
	return b.pub.Publish(ctx, value)
}

// logValue satisfies slog.LogValuer so Bot can be used in structured log lines.
var _ slog.LogValuer = (*Bot)(nil)

func (b *Bot) LogValue() slog.Value { return slog.StringValue("bot") }
