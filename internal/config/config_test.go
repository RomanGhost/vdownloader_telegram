package config

import (
	"reflect"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"BOT_TOKEN", "WORKER_URL", "KAFKA_BROKERS", "KAFKA_TOPIC", "KAFKA_JOBS_TOPIC"} {
		t.Setenv(key, "")
	}

	got := Load()
	want := Config{
		BotToken:       "",
		WorkerURL:      "http://localhost:8080",
		KafkaBrokers:   "localhost:9092",
		KafkaTopic:     "video.completed",
		KafkaJobsTopic: "video.jobs",
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadEnvOverridesDefaults(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("WORKER_URL", "http://worker:8080")
	t.Setenv("KAFKA_BROKERS", "b1:9092,b2:9092")

	got := Load()
	if got.BotToken != "test-token" {
		t.Errorf("BotToken = %q, want %q", got.BotToken, "test-token")
	}
	if got.WorkerURL != "http://worker:8080" {
		t.Errorf("WorkerURL = %q, want %q", got.WorkerURL, "http://worker:8080")
	}
	if got.KafkaBrokers != "b1:9092,b2:9092" {
		t.Errorf("KafkaBrokers = %q, want %q", got.KafkaBrokers, "b1:9092,b2:9092")
	}
}

func TestKafkaBrokersList(t *testing.T) {
	cases := []struct {
		brokers string
		want    []string
	}{
		{"localhost:9092", []string{"localhost:9092"}},
		{"b1:9092,b2:9092,b3:9092", []string{"b1:9092", "b2:9092", "b3:9092"}},
	}
	for _, tc := range cases {
		cfg := Config{KafkaBrokers: tc.brokers}
		got := cfg.KafkaBrokersList()
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Config{KafkaBrokers: %q}.KafkaBrokersList() = %v, want %v", tc.brokers, got, tc.want)
		}
	}
}
