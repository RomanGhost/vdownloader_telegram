package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"tgbot/internal/workerclient"
)

// startWebhookServer starts an HTTP server that receives CompletedEvent POSTs
// from the worker. It shuts down automatically when ctx is cancelled.
func (b *Bot) startWebhookServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", b.handleWebhook)

	srv := &http.Server{Addr: b.cfg.WebhookAddr, Handler: mux}

	go func() {
		b.log.Info("webhook server listening", "addr", b.cfg.WebhookAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			b.log.Error("webhook server error", "err", err)
		}
	}()

	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			b.log.Error("webhook server shutdown", "err", err)
		}
	}()
}

// handleWebhook receives a CompletedEvent from the worker and dispatches it.
// Returns 200 immediately; file delivery happens asynchronously so the worker
// is not blocked waiting for a potentially long Telegram upload.
func (b *Bot) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event workerclient.CompletedEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		b.log.Warn("webhook: bad payload", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	b.log.Info("webhook received", "job_id", event.JobID, "status", event.Status)
	w.WriteHeader(http.StatusOK)

	// Use a fresh context so the upload is not cancelled when the HTTP request ends.
	go b.processCompleted(context.Background(), event)
}

func (b *Bot) processCompleted(ctx context.Context, event workerclient.CompletedEvent) {
	b.mu.Lock()
	job, ok := b.jobs[event.JobID]
	if ok {
		delete(b.jobs, event.JobID)
	}
	b.mu.Unlock()

	if !ok {
		b.log.Warn("completed event for unknown job", "job_id", event.JobID)
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

	if event.Status != "ready" {
		editText(fmt.Sprintf("<b>%s</b>\n\nDownload failed: %s",
			escapeHTML(job.Title), escapeHTML(event.Error)))
		return
	}

	editText(fmt.Sprintf("<b>%s</b>\n\nDone! Sending file…", escapeHTML(job.Title)))

	if err := b.sendFile(ctx, job.ChatID, event.FileID, job.Title, job.AudioOnly); err != nil {
		b.log.Error("send file", "job_id", event.JobID, "err", err)
		editText(fmt.Sprintf("<b>%s</b>\n\nReady but failed to send: %s",
			escapeHTML(job.Title), escapeHTML(err.Error())))
		return
	}

	editText(fmt.Sprintf("<b>%s</b>\n\nDone! ✓", escapeHTML(job.Title)))
}

// logValue satisfies slog.LogValuer so Bot can be used in structured log lines.
var _ slog.LogValuer = (*Bot)(nil)

func (b *Bot) LogValue() slog.Value { return slog.StringValue("bot") }
