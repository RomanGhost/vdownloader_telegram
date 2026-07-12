package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// BotToken is the Telegram bot API token.
	// Env: BOT_TOKEN
	BotToken string

	// WorkerURL is the base URL of the vdownloader worker HTTP server.
	// Used both for API calls (/api/*) and for file download links (/files/*).
	// Env: WORKER_URL  Default: http://localhost:8080
	WorkerURL string

	// WebhookAddr is the address this bot's webhook HTTP server listens on.
	// The worker POSTs completion events to POST {host}/webhook.
	// Env: WEBHOOK_ADDR  Default: :8090
	WebhookAddr string
}

// Load reads .env (if present) and returns the populated Config.
func Load() Config {
	_ = godotenv.Load()
	return Config{
		BotToken:    getenv("BOT_TOKEN", ""),
		WorkerURL:   getenv("WORKER_URL", "http://localhost:8080"),
		WebhookAddr: getenv("WEBHOOK_ADDR", ":8090"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
