package bot

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"

	"tgbot/internal/workerclient"
)

func buildFormatKeyboard(formats []workerclient.FormatMessage) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for i, f := range formats {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: formatLabel(f), CallbackData: fmt.Sprintf("fmt:%d", i)},
		})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// formatLabel builds a human-readable button label for a yt-dlp format.
func formatLabel(f workerclient.FormatMessage) string {
	var parts []string

	quality := f.FormatNote
	if quality == "" {
		quality = f.Resolution
	}
	if quality == "" {
		quality = "audio"
	}
	parts = append(parts, quality)

	if f.Ext != "" {
		parts = append(parts, f.Ext)
	}

	switch {
	case f.Filesize > 0:
		parts = append(parts, fmt.Sprintf("%.1f MiB", float64(f.Filesize)/(1024*1024)))
	case f.TBR > 0:
		parts = append(parts, fmt.Sprintf("%.0f kbps", f.TBR))
	}

	return strings.Join(parts, " • ")
}

// formatToRequest builds a DownloadRequest for the chosen format.
func formatToRequest(url, title string, f workerclient.FormatMessage) workerclient.DownloadRequest {
	req := workerclient.DownloadRequest{
		URL:          url,
		Title:        title,
		QualityLabel: qualityLabel(f),
		AudioOnly:    isAudioOnly(f),
	}
	if isVideoOnly(f) {
		// Mux the video-only stream with the best available audio.
		req.FormatArg = f.FormatID + "+bestaudio"
		req.OutputFormat = "mp4"
	} else {
		req.FormatArg = f.FormatID
	}
	return req
}

// isAudioOnly reports whether the format carries audio but no video.
func isAudioOnly(f workerclient.FormatMessage) bool {
	return f.HaveAudio && !f.HaveVideo
}

// isVideoOnly reports whether the format carries video but no audio.
func isVideoOnly(f workerclient.FormatMessage) bool {
	return f.HaveVideo && !f.HaveAudio
}

func qualityLabel(f workerclient.FormatMessage) string {
	if f.FormatNote != "" {
		return f.FormatNote
	}
	if f.Resolution != "" {
		return f.Resolution
	}
	return f.FormatID
}
