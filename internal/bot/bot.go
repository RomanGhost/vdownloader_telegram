// Package bot implements the Telegram bot that interacts with the vdownloader worker.
//
// User flow:
//  1. User sends a video URL.
//  2. Bot calls GetFormats RPC → shows an inline keyboard with format presets.
//  3. User picks a format → bot calls Download RPC → edits message to "Downloading…".
//  4. Worker publishes a CompletedEvent → bot downloads the file from the file server
//     and sends it to the user, or reports an error.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/go-telegram/bot"

	"tgbot/internal/amqpclient"
	"tgbot/internal/config"
)

// userState holds a URL, title and available formats while the user is choosing.
type userState struct {
	URL     string
	Title   string
	Formats []amqpclient.FormatMessage
}

// pendingJob tracks a running download so the completed event handler can notify the user.
type pendingJob struct {
	ChatID    int64
	MsgID     int
	Title     string
	AudioOnly bool
}

// Bot is the Telegram bot wired to the vdownloader worker via RabbitMQ.
type Bot struct {
	tg     *bot.Bot
	worker *amqpclient.Client
	cfg    config.Config
	log    *slog.Logger

	mu     sync.Mutex
	states map[int64]*userState  // chatID → awaiting format selection
	jobs   map[int64]*pendingJob // jobID  → download in progress
}

// New creates the Bot, connects to Telegram and RabbitMQ, and registers all handlers.
func New(cfg config.Config, log *slog.Logger) (*Bot, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is not set")
	}

	b := &Bot{
		cfg:    cfg,
		log:    log,
		states: make(map[int64]*userState),
		jobs:   make(map[int64]*pendingJob),
	}

	worker, err := amqpclient.New(cfg.AMQPURL, log)
	if err != nil {
		return nil, fmt.Errorf("connect to worker: %w", err)
	}
	b.worker = worker

	tg, err := bot.New(cfg.BotToken,
		bot.WithDefaultHandler(b.onURL),
	)
	if err != nil {
		worker.Close()
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	b.tg = tg

	b.registerHandlers()
	return b, nil
}

// Run starts polling Telegram and consuming completion events.
// It blocks until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	events, err := b.worker.ConsumeCompleted(ctx)
	if err != nil {
		b.log.Error("failed to subscribe to completed events", "err", err)
	} else {
		go b.handleCompletedEvents(ctx, events)
	}

	b.tg.Start(ctx)
}

// Close releases AMQP resources.
func (b *Bot) Close() {
	b.worker.Close()
}

func (b *Bot) registerHandlers() {
	b.tg.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, b.onStart)
	b.tg.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, b.onStart)
	// Callback data has the form "fmt:N" where N is the preset index.
	b.tg.RegisterHandler(bot.HandlerTypeCallbackQueryData, "fmt:", bot.MatchTypePrefix, b.onCallback)
}
