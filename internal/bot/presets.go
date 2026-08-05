package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
)

// buildQualityKeyboard renders step 1: one button per available video
// quality tier plus a trailing "Audio only" entry.
func buildQualityKeyboard(videoHeights []int) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for _, h := range videoHeights {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: heightLabel(h), CallbackData: fmt.Sprintf("q:%d", h)},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "🎵 Audio only", CallbackData: "q:audio"},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// buildVideoAudioKeyboard renders step 2 for the video branch: with or
// without an audio track muxed in.
func buildVideoAudioKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "🔊 With audio", CallbackData: "va:1"}},
		{{Text: "🔇 Without audio", CallbackData: "va:0"}},
	}}
}

// buildAudioFormatKeyboard renders step 2 for the audio branch: one button
// per target codec, first entry marked as the default.
func buildAudioFormatKeyboard(audioFormats []string) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for i, f := range audioFormats {
		label := strings.ToUpper(f)
		if i == 0 {
			label += " (default)"
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: label, CallbackData: "af:" + f},
		})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// heightLabel renders a quality tier button label, using the "4K" shorthand
// for 2160p.
func heightLabel(height int) string {
	if height == 2160 {
		return "4K (2160p)"
	}
	return strconv.Itoa(height) + "p"
}
