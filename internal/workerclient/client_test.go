package workerclient

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGetFormatsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("url"); got != "https://example.com/video" {
			t.Errorf("query url = %q, want %q", got, "https://example.com/video")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"title":"Some Video","duration":120,"video_heights":[1080,720],"audio_formats":["mp3","opus"]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := New(srv.URL, testLogger())
	got, err := c.GetFormats(context.Background(), "https://example.com/video")
	if err != nil {
		t.Fatalf("GetFormats: %v", err)
	}
	if got.Title != "Some Video" || got.Duration != 120 || len(got.VideoHeights) != 2 {
		t.Errorf("GetFormats result = %+v, unexpected fields", got)
	}
}

func TestGetFormatsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"unsupported URL"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := New(srv.URL, testLogger())
	_, err := c.GetFormats(context.Background(), "https://example.com/bad")
	if err == nil {
		t.Fatal("GetFormats returned no error for a 400 response")
	}
	if err.Error() != "unsupported URL" {
		t.Errorf("error = %q, want the worker's error message %q", err.Error(), "unsupported URL")
	}
}

func TestGetJobSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/jobs/abc-123" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/jobs/abc-123")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"file_id":"abc-123","status":"ready","download_url":"/files/abc-123"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := New(srv.URL, testLogger())
	got, err := c.GetJob(context.Background(), "abc-123")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status != "ready" || got.FileID != "abc-123" {
		t.Errorf("GetJob result = %+v, unexpected fields", got)
	}
}

func TestGetJobNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "job not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, testLogger())
	_, err := c.GetJob(context.Background(), "unknown-id")
	if err == nil {
		t.Fatal("GetJob returned no error for a 404 response")
	}
}
