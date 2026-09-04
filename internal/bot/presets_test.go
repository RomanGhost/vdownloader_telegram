package bot

import "testing"

func TestHeightLabel(t *testing.T) {
	cases := []struct {
		height int
		want   string
	}{
		{2160, "4K (2160p)"},
		{1080, "1080p"},
		{360, "360p"},
	}
	for _, tc := range cases {
		if got := heightLabel(tc.height); got != tc.want {
			t.Errorf("heightLabel(%d) = %q, want %q", tc.height, got, tc.want)
		}
	}
}

func TestBuildQualityKeyboard(t *testing.T) {
	kb := buildQualityKeyboard([]int{1080, 720})
	// One row per height, plus a trailing "Audio only" row.
	if len(kb.InlineKeyboard) != 3 {
		t.Fatalf("got %d rows, want 3", len(kb.InlineKeyboard))
	}
	if got := kb.InlineKeyboard[0][0].CallbackData; got != "q:1080" {
		t.Errorf("row 0 CallbackData = %q, want %q", got, "q:1080")
	}
	if got := kb.InlineKeyboard[1][0].CallbackData; got != "q:720" {
		t.Errorf("row 1 CallbackData = %q, want %q", got, "q:720")
	}
	last := kb.InlineKeyboard[2][0]
	if last.CallbackData != "q:audio" {
		t.Errorf("last row CallbackData = %q, want %q", last.CallbackData, "q:audio")
	}
}

func TestBuildQualityKeyboardNoHeights(t *testing.T) {
	// A source with nothing at/below the smallest standard tier still has
	// to offer the audio-only entry, never an empty keyboard.
	kb := buildQualityKeyboard(nil)
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("got %d rows, want 1 (audio-only only)", len(kb.InlineKeyboard))
	}
	if got := kb.InlineKeyboard[0][0].CallbackData; got != "q:audio" {
		t.Errorf("CallbackData = %q, want %q", got, "q:audio")
	}
}

func TestBuildAudioFormatKeyboard(t *testing.T) {
	kb := buildAudioFormatKeyboard([]string{"mp3", "opus"})
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("got %d rows, want 2", len(kb.InlineKeyboard))
	}
	if got := kb.InlineKeyboard[0][0].Text; got != "MP3 (default)" {
		t.Errorf("first row label = %q, want %q", got, "MP3 (default)")
	}
	if got := kb.InlineKeyboard[1][0].Text; got != "OPUS" {
		t.Errorf("second row label = %q, want %q", got, "OPUS")
	}
	if got := kb.InlineKeyboard[0][0].CallbackData; got != "af:mp3" {
		t.Errorf("first row CallbackData = %q, want %q", got, "af:mp3")
	}
}

func TestContainsInt(t *testing.T) {
	vals := []int{1080, 720, 480}
	if !containsInt(vals, 720) {
		t.Error("containsInt(vals, 720) = false, want true")
	}
	if containsInt(vals, 4320) {
		t.Error("containsInt(vals, 4320) = true, want false")
	}
	if containsInt(nil, 1080) {
		t.Error("containsInt(nil, 1080) = true, want false")
	}
}
