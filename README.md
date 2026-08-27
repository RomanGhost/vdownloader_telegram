# vdownloader_telegram

Telegram bot front end for [vdownloader_worker](../vdownloader_worker/README.md). Talks to the worker over both HTTP (format lookup, job status, file download) and RabbitMQ (job submission, completion notification) — see the [root README](../README.md#architecture) for how the three services fit together.

## User flow

1. User sends a video URL.
2. Bot calls `GET /api/formats` on the worker → shows an inline keyboard with the standardized quality tiers available for this video (up to 2160p/4K, capped to the source's real max) plus an **🎵 Audio only** entry.
3. **Video tier picked** → second keyboard: **🔊 With audio** / **🔇 Without audio**.
   **Audio only picked** → second keyboard: **MP3 (default)** / **M4A** / **Opus** / **WAV**.
4. Bot publishes the resulting job request to the `video.jobs` RabbitMQ queue → edits the message to "Downloading…".
5. Worker publishes the completed job's `file_id` to `video.completed` → bot fetches the outcome via `GET /api/jobs/{file_id}` and sends the file, or a direct download link if it exceeds Telegram's upload limit.

Implementation: [internal/bot/handlers.go](internal/bot/handlers.go) (step 1–4), [internal/bot/mq.go](internal/bot/mq.go) (step 5), [internal/bot/presets.go](internal/bot/presets.go) (keyboard builders), [internal/bot/deliver.go](internal/bot/deliver.go) (file delivery / size check).

## Configuration

Env vars, read via `.env` → environment:

| Env var | Default | Meaning |
|---|---|---|
| `BOT_TOKEN` | *(required)* | Telegram bot API token |
| `WORKER_URL` | `http://localhost:8080` | Worker's HTTP base URL — used for `/api/formats`, `/api/jobs/{file_id}`, `/files/{file_id}` |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection URL |

The bot **consumes** completion notifications from the `video.completed` queue and **publishes** job requests to `video.jobs`. Queue names are fixed constants (`internal/mq`), not configuration.

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

No inbound port needed — the bot only makes outbound calls (Telegram long-polling, worker HTTP, RabbitMQ). See the repo root [docker-compose.yml](../docker-compose.yml) to run it alongside the worker and RabbitMQ; it reads `BOT_TOKEN` from this directory's `.env` via `env_file:`.

## RabbitMQ contract

Shares the wire format documented in [vdownloader_worker/README.md#rabbitmq-contract](../vdownloader_worker/README.md#rabbitmq-contract). Go types live in [internal/workerclient/client.go](internal/workerclient/client.go) (`DownloadRequest`, published to `video.jobs`; `GetFormatsResponse`, read from `GET /api/formats`) and [internal/bot/mq.go](internal/bot/mq.go) (`completedMessage`, consumed from `video.completed`).

`DownloadRequest.Duration` is echoed straight from `GetFormatsResponse.Duration` (captured in `userState` when the format list is first fetched) so the worker can size its download timeout without a second `yt-dlp -J` call.

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
    │   ├── bot.go                  # Bot struct, handler registration, mq publisher/consumer setup
    │   ├── handlers.go              # /start, URL intake, quality/audio callback handlers
    │   ├── presets.go               # Inline-keyboard builders for both selection steps
    │   ├── presets_test.go
    │   ├── mq.go                    # Completion consumer + job-request publisher
    │   ├── deliver.go               # File delivery (size check, SendAudio/SendDocument)
    │   └── deliver_test.go
    ├── config/
    │   ├── config.go                # Env var loading
    │   └── config_test.go
    ├── mq/
    │   ├── mq.go                    # Queue names + durable-queue declare helper
    │   ├── publisher.go             # Reconnecting persistent-message publisher
    │   └── consumer.go              # Reconnecting queue consumer
    └── workerclient/
        ├── client.go                # HTTP client for the worker's REST API
        └── client_test.go
```

## Testing

```bash
go test ./...
```

No live Telegram bot, worker, or RabbitMQ needed:

- `internal/config/config_test.go` — env var defaults/overrides.
- `internal/bot/presets_test.go` — inline-keyboard builders (`buildQualityKeyboard` including the "no video tiers, audio-only still offered" case), `heightLabel`, `containsInt`.
- `internal/bot/deliver_test.go` — `extractFilename` (Content-Disposition parsing), `escapeHTML`.
- `internal/workerclient/client_test.go` — `GetFormats`/`GetJob` success and error paths against an `httptest.Server` standing in for the worker.

Not covered: the `bot.HandlerType*` callback handlers in `handlers.go` (they take the `go-telegram/bot` library's own types, e.g. `*models.Update`, which aren't practical to construct without a real bot session) and the consume loop in `mq.go`. Those paths are exercised by the [repo root's end-to-end smoke test](../README.md#testing) instead — though that test only covers the web UI's submission path, not a real Telegram interaction, so a manual check via the actual bot is still worthwhile before deploying a change that touches `handlers.go` or `mq.go`.
