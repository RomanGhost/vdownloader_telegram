package bot

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const maxTelegramBytes = 50 * 1024 * 1024 // 50 MiB

// sendFile delivers the downloaded file to the Telegram chat.
// It first performs a HEAD request to check the file size without transferring
// the body. If the file exceeds Telegram's 50 MiB limit a direct download link
// is sent instead.
func (b *Bot) sendFile(ctx context.Context, chatID int64, fileID, title string, audioOnly bool) error {
	fileURL := b.cfg.WorkerURL + "/files/" + fileID
	client := &http.Client{Timeout: 10 * time.Minute}

	// ── 1. Check file size with HEAD ──────────────────────────────────────────
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, fileURL, nil)
	if err != nil {
		return fmt.Errorf("build HEAD request: %w", err)
	}
	headResp, err := client.Do(headReq)
	if err != nil {
		return fmt.Errorf("HEAD file server: %w", err)
	}
	headResp.Body.Close()

	if headResp.StatusCode != http.StatusOK {
		return fmt.Errorf("file server returned %s", headResp.Status)
	}

	if headResp.ContentLength > maxTelegramBytes {
		sizeMiB := float64(headResp.ContentLength) / (1024 * 1024)
		_, err = b.tg.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text: fmt.Sprintf(
				"<b>%s</b>\n\nFile is too large for Telegram (%.1f MiB).\n"+
					"<a href=\"%s\">⬇️ Download directly</a>",
				escapeHTML(title), sizeMiB, fileURL),
			ParseMode: models.ParseModeHTML,
		})
		return err
	}

	// ── 2. Stream the file body to Telegram ───────────────────────────────────
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return fmt.Errorf("build GET request: %w", err)
	}
	resp, err := client.Do(getReq)
	if err != nil {
		return fmt.Errorf("fetch from file server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file server returned %s", resp.Status)
	}

	// Guard against servers that omit Content-Length on HEAD but include it on GET.
	if resp.ContentLength > maxTelegramBytes {
		sizeMiB := float64(resp.ContentLength) / (1024 * 1024)
		_, err = b.tg.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text: fmt.Sprintf(
				"<b>%s</b>\n\nFile is too large for Telegram (%.1f MiB).\n"+
					"<a href=\"%s\">⬇️ Download directly</a>",
				escapeHTML(title), sizeMiB, fileURL),
			ParseMode: models.ParseModeHTML,
		})
		return err
	}

	filename := extractFilename(resp.Header.Get("Content-Disposition"))
	if filename == "" {
		if audioOnly {
			filename = title + ".mp3"
		} else {
			filename = title + ".mp4"
		}
	}

	if audioOnly {
		_, err = b.tg.SendAudio(ctx, &bot.SendAudioParams{
			ChatID:  chatID,
			Audio:   &models.InputFileUpload{Filename: filename, Data: resp.Body},
			Caption: title,
		})
	} else {
		_, err = b.tg.SendDocument(ctx, &bot.SendDocumentParams{
			ChatID:   chatID,
			Document: &models.InputFileUpload{Filename: filename, Data: resp.Body},
			Caption:  title,
		})
	}
	return err
}

// extractFilename parses the filename from a Content-Disposition header value.
func extractFilename(contentDisposition string) string {
	if contentDisposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(contentDisposition)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// escapeHTML escapes characters special in Telegram's HTML parse mode.
func escapeHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(s)
}
