# vdownloader_telegram

Telegram bot front end for [vdownloader_worker](../vdownloader_worker/README.md). Talks to the worker over both HTTP (format lookup, job status, file download) and Kafka (job submission, completion notification) — see the [root README](../README.md#architecture) for how the three services fit together.

## User flow

1. User sends a video URL.
2. Bot calls `GET /api/formats` on the worker → shows an inline keyboard with the standardized quality tiers available for this video (up to 2160p/4K, capped to the source's real max) plus an **🎵 Audio only** entry.
3. **Video tier picked** → second keyboard: **🔊 With audio** / **🔇 Without audio**.
   **Audio only picked** → second keyboard: **MP3 (default)** / **M4A** / **Opus** / **WAV**.
4. Bot publishes the resulting job request to the worker's job-requests Kafka topic → edits the message to "Downloading…".
5. Worker publishes the completed job's `file_id` to Kafka → bot fetches the outcome via `GET /api/jobs/{file_id}` and sends the file, or a direct download link if it exceeds Telegram's 50 MiB upload limit.

Implementation: [internal/bot/handlers.go](internal/bot/handlers.go) (step 1–4), [internal/bot/kafka.go](internal/bot/kafka.go) (step 5), [internal/bot/presets.go](internal/bot/presets.go) (keyboard builders), [internal/bot/deliver.go](internal/bot/deliver.go) (file delivery / size check).

## Configuration

Env vars, read via `.env` → environment:

| Env var | Default | Meaning |
|---|---|---|
| `BOT_TOKEN` | *(required)* | Telegram bot API token |
| `WORKER_URL` | `http://localhost:8080` | Worker's HTTP base URL — used for `/api/formats`, `/api/jobs/{file_id}`, `/files/{file_id}` |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated broker list |
| `KAFKA_TOPIC` | `video.completed` | Topic the bot **consumes** completion notifications from |
| `KAFKA_JOBS_TOPIC` | `video.jobs` | Topic the bot **publishes** job requests to |

## Running

```bash
go build -o telegram_service .
BOT_TOKEN=... ./telegram_service
```

### Docker

```bash
docker build -t vdownloader-telegram .
docker run -it --env-file .env vdownloader-telegram
```

No inbound port needed — the bot only makes outbound calls (Telegram long-polling, worker HTTP, Kafka). See the repo root [docker-compose.yml](../docker-compose.yml) to run it alongside the worker and Kafka; it reads `BOT_TOKEN` from this directory's `.env` via `env_file:`.

## Kafka contract

Shares the wire format documented in [vdownloader_worker/README.md#kafka-contract](../vdownloader_worker/README.md#kafka-contract). Go types live in [internal/workerclient/client.go](internal/workerclient/client.go) (`DownloadRequest`, published to `KAFKA_JOBS_TOPIC`) and [internal/bot/kafka.go](internal/bot/kafka.go) (`completedMessage`, consumed from `KAFKA_TOPIC`).

The bot generates `file_id` itself (`uuid.NewString()`) when publishing a job request, and keeps an in-memory map `file_id → pendingJob` (chat/message to edit, title, whether it's an audio delivery) so it knows what to do when the matching completion event arrives. This map is not persisted — a bot restart loses track of jobs in flight; the worker still finishes them, but no one will notify the user.

## HTTP calls to the worker

| Call | Used for |
|---|---|
| `GET /api/formats?url=` | Step 2 of the user flow |
| `GET /api/jobs/{file_id}` | After a completion event, to learn `ready`/`failed`, error, and `file_id` for download |
| `GET`/`HEAD /files/{file_id}` | Delivering the file — `HEAD` first to check size against Telegram's 50 MiB cap before streaming |

## Project structure

```
.
├── main.go                        # Entry point
└── internal/
    ├── bot/
    │   ├── bot.go                  # Bot struct, handler registration, Kafka writer setup
    │   ├── handlers.go              # /start, URL intake, quality/audio callback handlers
    │   ├── presets.go               # Inline-keyboard builders for both selection steps
    │   ├── kafka.go                 # Completion consumer + job-request publisher
    │   └── deliver.go               # File delivery (size check, SendAudio/SendDocument)
    ├── config/
    │   └── config.go                # Env var loading
    └── workerclient/
        └── client.go                # HTTP client for the worker's REST API
```
