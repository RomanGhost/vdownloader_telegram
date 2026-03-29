package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"tgbot/internal/amqpclient"
)

func (b *Bot) onStart(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	tg.SendMessage(ctx, &bot.SendMessageParams{ //nolint:errcheck
		ChatID: update.Message.Chat.ID,
		Text: "Send me a video URL and I will download it.\n\n" +
			"Supported: YouTube, Vimeo, Twitter, TikTok, and many more sites via yt-dlp.",
	})
}

// onURL is the default handler — it processes every text message that is not a command.
func (b *Bot) onURL(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	url := strings.TrimSpace(update.Message.Text)
	chatID := update.Message.Chat.ID

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		tg.SendMessage(ctx, &bot.SendMessageParams{ //nolint:errcheck
			ChatID: chatID,
			Text:   "Please send a valid URL starting with http:// or https://",
		})
		return
	}

	// Send a placeholder we can edit in-place instead of accumulating messages.
	sent, err := tg.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Fetching video info...",
	})
	if err != nil {
		b.log.Error("send message", "err", err)
		return
	}

	resp, err := b.worker.GetFormats(ctx, url)
	if err != nil {
		tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
			ChatID:    chatID,
			MessageID: sent.ID,
			Text:      fmt.Sprintf("Failed to get video info:\n%s", err),
		})
		return
	}

	b.mu.Lock()
	b.states[chatID] = &userState{URL: url, Title: resp.Title}
	b.mu.Unlock()

	tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
		ChatID:      chatID,
		MessageID:   sent.ID,
		Text:        fmt.Sprintf("*%s*\n\nSelect a format:", escapeMarkdown(resp.Title)),
		ParseMode:   models.ParseModeMarkdown,
		ReplyMarkup: buildFormatKeyboard(),
	})
}

// onCallback handles format-selection button taps.
func (b *Bot) onCallback(ctx context.Context, tg *bot.Bot, update *models.Update) {
	cbq := update.CallbackQuery
	if cbq == nil || cbq.Message.Message == nil {
		return
	}

	chatID := cbq.Message.Message.Chat.ID
	msgID := cbq.Message.Message.ID

	// Parse preset index from callback data "fmt:N".
	idx, err := strconv.Atoi(strings.TrimPrefix(cbq.Data, "fmt:"))
	if err != nil || idx < 0 || idx >= len(presets) {
		tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{ //nolint:errcheck
			CallbackQueryID: cbq.ID,
			Text:            "Unknown format",
		})
		return
	}

	b.mu.Lock()
	state, ok := b.states[chatID]
	b.mu.Unlock()

	if !ok {
		tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{ //nolint:errcheck
			CallbackQueryID: cbq.ID,
			Text:            "Session expired. Please send the URL again.",
			ShowAlert:       true,
		})
		return
	}

	// Acknowledge the tap immediately so Telegram removes the loading spinner.
	tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cbq.ID}) //nolint:errcheck

	p := presets[idx]

	tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
		ChatID:    chatID,
		MessageID: msgID,
		Text:      fmt.Sprintf("*%s*\n\nQueuing download: %s...", escapeMarkdown(state.Title), p.Label),
		ParseMode: models.ParseModeMarkdown,
	})

	dlCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	dlResp, err := b.worker.Download(dlCtx, amqpclient.DownloadRequest{
		URL:          state.URL,
		Title:        state.Title,
		FormatArg:    p.Arg,
		QualityLabel: p.QualityLabel,
		AudioOnly:    p.AudioOnly,
		MergeAudio:   p.MergeAudio,
		OutputFormat: p.OutputFormat,
	})
	if err != nil {
		tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
			ChatID:    chatID,
			MessageID: msgID,
			Text:      fmt.Sprintf("*%s*\n\nFailed to queue download:\n%s", escapeMarkdown(state.Title), err),
			ParseMode: models.ParseModeMarkdown,
		})
		return
	}

	b.mu.Lock()
	b.jobs[dlResp.JobID] = &pendingJob{
		ChatID:    chatID,
		MsgID:     msgID,
		Title:     state.Title,
		AudioOnly: p.AudioOnly,
	}
	delete(b.states, chatID)
	b.mu.Unlock()

	tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
		ChatID:    chatID,
		MessageID: msgID,
		Text:      fmt.Sprintf("*%s*\n\nDownloading %s (job #%d)...", escapeMarkdown(state.Title), p.Label, dlResp.JobID),
		ParseMode: models.ParseModeMarkdown,
	})
}
