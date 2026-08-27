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

	// RabbitURL is the RabbitMQ connection URL. The bot publishes job requests
	// to the "video.jobs" queue and consumes completions from "video.completed"
	// (queue names are constants in internal/mq).
	// Env: RABBITMQ_URL  Default: amqp://guest:guest@localhost:5672/
	RabbitURL string
}

// Load reads .env (if present) and returns the populated Config.
func Load() Config {
	_ = godotenv.Load()
	return Config{
		BotToken:  getenv("BOT_TOKEN", ""),
		WorkerURL: getenv("WORKER_URL", "http://localhost:8080"),
		RabbitURL: getenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
