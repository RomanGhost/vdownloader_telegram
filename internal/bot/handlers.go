package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"tgbot/internal/workerclient"
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

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "www.") {
		tg.SendMessage(ctx, &bot.SendMessageParams{ //nolint:errcheck
			ChatID: chatID,
			Text:   "Please send a valid URL starting with http:// or https://",
		})
		return
	}

	// Send a placeholder we can edit in-place instead of accumulating messages.
	sent, err := tg.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "Fetching video info…",
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
	b.states[chatID] = &userState{
		URL:          url,
		Title:        resp.Title,
		VideoHeights: resp.VideoHeights,
		AudioFormats: resp.AudioFormats,
	}
	b.mu.Unlock()

	if _, err := tg.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   sent.ID,
		Text:        fmt.Sprintf("<b>%s</b>\n\nSelect a quality:", escapeHTML(resp.Title)),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: buildQualityKeyboard(resp.VideoHeights),
	}); err != nil {
		b.log.Error("edit message with keyboard", "chat_id", chatID, "err", err)
	}
}

// onQualityCallback handles step 1: a video quality tier or the "Audio only" pick.
func (b *Bot) onQualityCallback(ctx context.Context, tg *bot.Bot, update *models.Update) {
	cbq := update.CallbackQuery
	if cbq == nil || cbq.Message.Message == nil {
		return
	}
	chatID := cbq.Message.Message.Chat.ID
	msgID := cbq.Message.Message.ID

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

	data := strings.TrimPrefix(cbq.Data, "q:")

	if data == "audio" {
		tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cbq.ID}) //nolint:errcheck
		tg.EditMessageText(ctx, &bot.EditMessageTextParams{                                  //nolint:errcheck
			ChatID:      chatID,
			MessageID:   msgID,
			Text:        fmt.Sprintf("<b>%s</b>\n\nSelect an audio format:", escapeHTML(state.Title)),
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: buildAudioFormatKeyboard(state.AudioFormats),
		})
		return
	}

	height, err := strconv.Atoi(data)
	if err != nil || !containsInt(state.VideoHeights, height) {
		tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{ //nolint:errcheck
			CallbackQueryID: cbq.ID,
			Text:            "Unknown quality",
		})
		return
	}
	tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cbq.ID}) //nolint:errcheck

	b.mu.Lock()
	state.PendingHeight = height
	b.mu.Unlock()

	tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        fmt.Sprintf("<b>%s</b>\n\n%s — with or without audio?", escapeHTML(state.Title), heightLabel(height)),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: buildVideoAudioKeyboard(),
	})
}

// onVideoAudioCallback handles step 2 of the video branch: with or without audio.
func (b *Bot) onVideoAudioCallback(ctx context.Context, tg *bot.Bot, update *models.Update) {
	cbq := update.CallbackQuery
	if cbq == nil || cbq.Message.Message == nil {
		return
	}
	chatID := cbq.Message.Message.Chat.ID
	msgID := cbq.Message.Message.ID

	b.mu.Lock()
	state, ok := b.states[chatID]
	b.mu.Unlock()
	if !ok || state.PendingHeight == 0 {
		tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{ //nolint:errcheck
			CallbackQueryID: cbq.ID,
			Text:            "Session expired. Please send the URL again.",
			ShowAlert:       true,
		})
		return
	}
	tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cbq.ID}) //nolint:errcheck

	withAudio := strings.TrimPrefix(cbq.Data, "va:") == "1"
	label := heightLabel(state.PendingHeight)
	if !withAudio {
		label += " (no audio)"
	}

	tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
		ChatID:    chatID,
		MessageID: msgID,
		Text:      fmt.Sprintf("<b>%s</b>\n\nQueuing download: %s…", escapeHTML(state.Title), label),
		ParseMode: models.ParseModeHTML,
	})

	req := workerclient.DownloadRequest{
		FileID:    uuid.NewString(),
		URL:       state.URL,
		Title:     state.Title,
		Kind:      "video",
		Height:    state.PendingHeight,
		WithAudio: withAudio,
	}
	b.queueJob(ctx, tg, chatID, msgID, state, req, false, label)
}

// onAudioFormatCallback handles step 2 of the audio branch: the target codec.
func (b *Bot) onAudioFormatCallback(ctx context.Context, tg *bot.Bot, update *models.Update) {
	cbq := update.CallbackQuery
	if cbq == nil || cbq.Message.Message == nil {
		return
	}
	chatID := cbq.Message.Message.Chat.ID
	msgID := cbq.Message.Message.ID

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
	tg.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cbq.ID}) //nolint:errcheck

	audioFormat := strings.TrimPrefix(cbq.Data, "af:")
	label := strings.ToUpper(audioFormat) + " audio"

	tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
		ChatID:    chatID,
		MessageID: msgID,
		Text:      fmt.Sprintf("<b>%s</b>\n\nQueuing download: %s…", escapeHTML(state.Title), label),
		ParseMode: models.ParseModeHTML,
	})

	req := workerclient.DownloadRequest{
		FileID:      uuid.NewString(),
		URL:         state.URL,
		Title:       state.Title,
		Kind:        "audio",
		AudioFormat: audioFormat,
	}
	b.queueJob(ctx, tg, chatID, msgID, state, req, true, label)
}

// queueJob publishes req to Kafka, registers the pending job, clears the
// user's state, and edits the status message to reflect progress. Shared by
// the video and audio branches once they've built their DownloadRequest.
func (b *Bot) queueJob(
	ctx context.Context, tg *bot.Bot, chatID int64, msgID int,
	state *userState, req workerclient.DownloadRequest, audioOnly bool, label string,
) {
	dlCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := b.publishJobRequest(dlCtx, req); err != nil {
		tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
			ChatID:    chatID,
			MessageID: msgID,
			Text:      fmt.Sprintf("<b>%s</b>\n\nFailed to queue download:\n%s", escapeHTML(state.Title), err),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	b.mu.Lock()
	b.jobs[req.FileID] = &pendingJob{ChatID: chatID, MsgID: msgID, Title: state.Title, AudioOnly: audioOnly}
	delete(b.states, chatID)
	b.mu.Unlock()

	tg.EditMessageText(ctx, &bot.EditMessageTextParams{ //nolint:errcheck
		ChatID:    chatID,
		MessageID: msgID,
		Text:      fmt.Sprintf("<b>%s</b>\n\nDownloading %s…", escapeHTML(state.Title), label),
		ParseMode: models.ParseModeHTML,
	})
}

func containsInt(vals []int, v int) bool {
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}
