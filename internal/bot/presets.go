package bot

import (
	"fmt"

	"github.com/go-telegram/bot/models"
)

type formatPreset struct {
	Label        string
	Arg          string // yt-dlp -f argument
	QualityLabel string
	AudioOnly    bool
	MergeAudio   bool
	OutputFormat string // force container, empty = keep original
}

var presets = []formatPreset{
	{"Best quality (video+audio)", "bestvideo+bestaudio/best", "best quality", false, false, "mp4"},
	{"Audio only (best quality)", "bestaudio/best", "audio only", true, false, ""},
	{"1080p + audio", "bestvideo[height<=1080]+bestaudio/best[height<=1080]", "1080p", false, false, "mp4"},
	{"720p + audio", "bestvideo[height<=720]+bestaudio/best[height<=720]", "720p", false, false, "mp4"},
	{"480p + audio", "bestvideo[height<=480]+bestaudio/best[height<=480]", "480p", false, false, "mp4"},
}

func buildFormatKeyboard() *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for i, p := range presets {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: p.Label, CallbackData: fmt.Sprintf("fmt:%d", i)},
		})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
