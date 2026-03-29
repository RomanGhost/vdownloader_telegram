package bot

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"tgbot/internal/amqpclient"
)

func (b *Bot) handleCompletedEvents(ctx context.Context, events <-chan amqpclient.CompletedEvent) {
	for event := range events {
		b.processCompleted(ctx, event)
	}
}

func (b *Bot) processCompleted(ctx context.Context, event amqpclient.CompletedEvent) {
	b.mu.Lock()
	job, ok := b.jobs[event.JobID]
	if ok {
		delete(b.jobs, event.JobID)
	}
	b.mu.Unlock()

	if !ok {
		b.log.Warn("received completed event for unknown job", "job_id", event.JobID)
		return
	}

	editText := func(text string) {
		if _, err := b.tg.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    job.ChatID,
			MessageID: job.MsgID,
			Text:      text,
			ParseMode: models.ParseModeMarkdown,
		}); err != nil {
			b.log.Warn("edit status message", "err", err)
		}
	}

	if event.Status != "ready" {
		editText(fmt.Sprintf("*%s*\n\nDownload failed: %s",
			escapeMarkdown(job.Title), escapeMarkdown(event.Error)))
		return
	}

	editText(fmt.Sprintf("*%s*\n\nDone! Sending file...", escapeMarkdown(job.Title)))

	if err := b.sendFile(ctx, job.ChatID, event.FileID, job.Title, job.AudioOnly); err != nil {
		b.log.Error("send file", "job_id", event.JobID, "err", err)
		editText(fmt.Sprintf("*%s*\n\nDownload is ready but failed to send file:\n%s",
			escapeMarkdown(job.Title), escapeMarkdown(err.Error())))
		return
	}

	editText(fmt.Sprintf("*%s*\n\nDone!", escapeMarkdown(job.Title)))
}
