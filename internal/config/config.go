package config

import (
	"os"
	"strings"

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

	// KafkaBrokers is a comma-separated list of Kafka broker addresses.
	// Env: KAFKA_BROKERS  Default: localhost:9092
	KafkaBrokers string

	// KafkaTopic is the topic job completion notifications are read from.
	// Env: KAFKA_TOPIC  Default: video.completed
	KafkaTopic string

	// KafkaJobsTopic is the topic download job requests are published to.
	// Env: KAFKA_JOBS_TOPIC  Default: video.jobs
	KafkaJobsTopic string
}

// KafkaBrokersList splits KafkaBrokers into individual broker addresses.
func (c Config) KafkaBrokersList() []string {
	return strings.Split(c.KafkaBrokers, ",")
}

// Load reads .env (if present) and returns the populated Config.
func Load() Config {
	_ = godotenv.Load()
	return Config{
		BotToken:       getenv("BOT_TOKEN", ""),
		WorkerURL:      getenv("WORKER_URL", "http://localhost:8080"),
		KafkaBrokers:   getenv("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:     getenv("KAFKA_TOPIC", "video.completed"),
		KafkaJobsTopic: getenv("KAFKA_JOBS_TOPIC", "video.jobs"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
