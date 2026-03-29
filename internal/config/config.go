package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	BotToken      string // Telegram bot token (BOT_TOKEN)
	AMQPUrl       string // RabbitMQ connection URL (AMQP_URL)
	FileServerURL string // Base URL of the worker's file server (FILE_SERVER_URL)
}

// Load reads .env (if present) and returns the populated Config.
func Load() Config {
	_ = godotenv.Load()
	return Config{
		BotToken:      getenv("BOT_TOKEN", ""),
		AMQPUrl:       getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		FileServerURL: getenv("FILE_SERVER_URL", "http://localhost:8080"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
