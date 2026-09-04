package bot

import "testing"

func TestExtractFilename(t *testing.T) {
	cases := []struct {
		name                string
		contentDisposition  string
		want                string
	}{
		{"empty header", "", ""},
		{"quoted filename", `attachment; filename="My Video [1080p].mp4"`, "My Video [1080p].mp4"},
		{"malformed header", "not a valid header;;;", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractFilename(tc.contentDisposition); got != tc.want {
				t.Errorf("extractFilename(%q) = %q, want %q", tc.contentDisposition, got, tc.want)
			}
		})
	}
}

func TestEscapeHTML(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"plain text", "plain text"},
		{"<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"Tom & Jerry", "Tom &amp; Jerry"},
	}
	for _, tc := range cases {
		if got := escapeHTML(tc.input); got != tc.want {
			t.Errorf("escapeHTML(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
