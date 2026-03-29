package amqpclient

// Queue names must match the worker.
const (
	queueGetFormats = "video.get_formats"
	queueDownload   = "video.download"
	queueCompleted  = "video.completed"
)

// ── Message types (mirrors worker's queue/messages.go) ──────────────────────

type GetFormatsRequest struct {
	URL string `json:"url"`
}

type FormatMessage struct {
	FormatID      string  `json:"format_id"`
	Ext           string  `json:"ext"`
	Resolution    string  `json:"resolution"`
	FPS           float64 `json:"fps"`
	TBR           float64 `json:"tbr"`
	VCodec        string  `json:"vcodec"`
	AudioChannels int     `json:"audio_channels"`
	Filesize      int64   `json:"filesize"`
	FormatNote    string  `json:"format_note"`
	AudioOnly     bool    `json:"audio_only"`
	VideoOnly     bool    `json:"video_only"`
}

type GetFormatsResponse struct {
	Title   string          `json:"title"`
	Formats []FormatMessage `json:"formats"`
	Error   string          `json:"error,omitempty"`
}

type DownloadRequest struct {
	URL          string `json:"url"`
	Title        string `json:"title"`
	FormatArg    string `json:"format_arg"`
	QualityLabel string `json:"quality_label"`
	AudioOnly    bool   `json:"audio_only"`
	MergeAudio   bool   `json:"merge_audio"`
	OutputFormat string `json:"output_format,omitempty"`
}

type DownloadResponse struct {
	JobID int64  `json:"job_id"`
	Error string `json:"error,omitempty"`
}

type CompletedEvent struct {
	JobID  int64  `json:"job_id"`
	FileID string `json:"file_id,omitempty"`
	Status string `json:"status"` // "ready" | "failed"
	Error  string `json:"error,omitempty"`
}
