// Package bot implements the Telegram bot that interacts with the vdownloader worker.
//
// User flow:
//  1. User sends a video URL.
//  2. Bot calls GET /api/formats → shows an inline keyboard with format presets.
//  3. User picks a format → bot calls POST /api/jobs → edits message to "Downloading…".
//  4. Worker POSTs a CompletedEvent to the bot's webhook → bot sends the file (or a
//     direct download link when the file exceeds Telegram's 50 MiB limit).
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/go-telegram/bot"

	"tgbot/internal/config"
	"tgbot/internal/workerclient"
)

// userState holds a URL, title and available formats while the user is choosing.
type userState struct {
	URL     string
	Title   string
	Formats []workerclient.FormatMessage
}

// pendingJob tracks a running download so the completed event handler can notify the user.
type pendingJob struct {
	ChatID    int64
	MsgID     int
	Title     string
	AudioOnly bool
}

// Bot is the Telegram bot wired to the vdownloader worker via HTTP.
type Bot struct {
	tg     *bot.Bot
	worker *workerclient.Client
	cfg    config.Config
	log    *slog.Logger

	mu     sync.Mutex
	states map[int64]*userState  // chatID → awaiting format selection
	jobs   map[int64]*pendingJob // jobID  → download in progress
}

// New creates the Bot, connects to Telegram, and registers all handlers.
func New(cfg config.Config, log *slog.Logger) (*Bot, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is not set")
	}

	b := &Bot{
		cfg:    cfg,
		log:    log,
		worker: workerclient.New(cfg.WorkerURL, log),
		states: make(map[int64]*userState),
		jobs:   make(map[int64]*pendingJob),
	}

	tg, err := bot.New(cfg.BotToken,
		bot.WithDefaultHandler(b.onURL),
	)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	b.tg = tg
	b.registerHandlers()
	return b, nil
}

// Run starts the webhook HTTP server and Telegram polling.
// It blocks until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	b.startWebhookServer(ctx)
	b.tg.Start(ctx)
}

// Close is a no-op kept for API compatibility; the webhook server shuts down
// via context cancellation in Run.
func (b *Bot) Close() {}

func (b *Bot) registerHandlers() {
	b.tg.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, b.onStart)
	b.tg.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, b.onStart)
	// Callback data has the form "fmt:N" where N is the preset index.
	b.tg.RegisterHandler(bot.HandlerTypeCallbackQueryData, "fmt:", bot.MatchTypePrefix, b.onCallback)
}
