// Package workerclient calls the vdownloader worker HTTP API.
//
// Replaces the old RabbitMQ RPC client: every call is a plain HTTP request
// against the worker's REST endpoints.
package workerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// ── Message types (must stay in sync with the worker's api package) ──────────

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
	HaveAudio     bool    `json:"have_audio"`
	HaveVideo     bool    `json:"have_video"`
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

// CompletedEvent is POSTed to the bot's webhook by the worker on job completion.
type CompletedEvent struct {
	JobID  int64  `json:"job_id"`
	FileID string `json:"file_id,omitempty"`
	Status string `json:"status"` // "ready" | "failed"
	Error  string `json:"error,omitempty"`
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client calls the worker REST API.
type Client struct {
	baseURL string
	http    *http.Client
	log     *slog.Logger
}

// New creates a Client targeting the worker at baseURL (e.g. "http://localhost:8080").
func New(baseURL string, log *slog.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 90 * time.Second},
		log:     log,
	}
}

// GetFormats fetches available formats for the given video URL.
func (c *Client) GetFormats(ctx context.Context, videoURL string) (*GetFormatsResponse, error) {
	endpoint := c.baseURL + "/api/formats?url=" + url.QueryEscape(videoURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get formats: %w", err)
	}
	defer resp.Body.Close()

	var result GetFormatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || result.Error != "" {
		msg := result.Error
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("%s", msg)
	}

	c.log.Info("got formats", "url", videoURL, "title", result.Title, "count", len(result.Formats))
	return &result, nil
}

// Download submits a download job and returns immediately with the job ID.
// The worker runs the actual download asynchronously and POSTs to the webhook on completion.
func (c *Client) Download(ctx context.Context, req DownloadRequest) (*DownloadResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/jobs", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post job: %w", err)
	}
	defer resp.Body.Close()

	var result DownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	c.log.Info("job queued", "job_id", result.JobID, "url", req.URL)
	return &result, nil
}
