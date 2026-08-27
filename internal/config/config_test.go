package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"BOT_TOKEN", "WORKER_URL", "RABBITMQ_URL"} {
		t.Setenv(key, "")
	}

	got := Load()
	want := Config{
		BotToken:  "",
		WorkerURL: "http://localhost:8080",
		RabbitURL: "amqp://guest:guest@localhost:5672/",
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadEnvOverridesDefaults(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("WORKER_URL", "http://worker:8080")
	t.Setenv("RABBITMQ_URL", "amqp://user:pass@broker:5672/")

	got := Load()
	if got.BotToken != "test-token" {
		t.Errorf("BotToken = %q, want %q", got.BotToken, "test-token")
	}
	if got.WorkerURL != "http://worker:8080" {
		t.Errorf("WorkerURL = %q, want %q", got.WorkerURL, "http://worker:8080")
	}
	if got.RabbitURL != "amqp://user:pass@broker:5672/" {
		t.Errorf("RabbitURL = %q, want %q", got.RabbitURL, "amqp://user:pass@broker:5672/")
	}
}
