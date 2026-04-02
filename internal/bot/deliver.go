package bot

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// sendFile downloads the file from the worker's file server and uploads it to
// the Telegram chat. If the file exceeds the 50 MiB bot API limit, it sends
// the direct download URL as a fallback.
func (b *Bot) sendFile(ctx context.Context, chatID int64, fileID, title string, audioOnly bool) error {
	fileURL := b.cfg.FileServerURL + "/files/" + fileID

	resp, err := http.Get(fileURL) //nolint:gosec // URL is config + server-supplied UUID
	if err != nil {
		return fmt.Errorf("fetch from file server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file server returned %s", resp.Status)
	}

	const maxTelegramBytes = 50 * 1024 * 1024
	if resp.ContentLength > 0 && resp.ContentLength > maxTelegramBytes {
		sizeMiB := float64(resp.ContentLength) / (1024 * 1024)
		_, err = b.tg.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text: fmt.Sprintf("File is too large for Telegram (%.1f MiB).\nDownload directly:\n%s",
				sizeMiB, fileURL),
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
