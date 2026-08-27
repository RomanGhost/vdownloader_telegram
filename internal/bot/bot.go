// Package bot implements the Telegram bot that interacts with the vdownloader worker.
//
// User flow:
//  1. User sends a video URL.
//  2. Bot calls GET /api/formats → shows an inline keyboard with the standardized
//     quality tiers available for this video (up to 2160p/4K, capped to the
//     source's real max) plus an "Audio only" entry.
//  3. Video tier picked → second keyboard: with audio / without audio.
//     Audio only picked → second keyboard: mp3 (default) / m4a / opus / wav.
//  4. Bot publishes the resulting job request to the "video.jobs" RabbitMQ
//     queue → edits message to "Downloading…".
//  5. Worker publishes the completed job's file_id to "video.completed" → bot
//     fetches the outcome via GET /api/jobs/{file_id} and sends the file (or a
//     direct download link when the file exceeds Telegram's 50 MiB limit).
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/go-telegram/bot"

	"tgbot/internal/config"
	"tgbot/internal/mq"
	"tgbot/internal/workerclient"
)

// userState holds a URL, title and available formats while the user is
// choosing. PendingHeight is set once the user picks a video quality tier,
// while the bot awaits the follow-up with-audio/without-audio choice.
type userState struct {
	URL           string
	Title         string
	Duration      float64 // seconds; 0 when the source doesn't report it
	VideoHeights  []int
	AudioFormats  []string
	PendingHeight int
}

// pendingJob tracks a running download so the completed event handler can notify the user.
type pendingJob struct {
	ChatID    int64
	MsgID     int
	Title     string
	AudioOnly bool
}

// Bot is the Telegram bot wired to the vdownloader worker via RabbitMQ (job
// submission and completion) and HTTP (formats lookup, job status, file download).
type Bot struct {
	tg     *bot.Bot
	worker *workerclient.Client
	pub    *mq.Publisher // publishes job requests to "video.jobs"
	sub    *mq.Consumer  // consumes completions from "video.completed"
	cfg    config.Config
	log    *slog.Logger

	mu     sync.Mutex
	states map[int64]*userState   // chatID  → awaiting format selection
	jobs   map[string]*pendingJob // file_id → download in progress
}

// New creates the Bot, connects to Telegram and RabbitMQ, and registers all handlers.
func New(cfg config.Config, log *slog.Logger) (*Bot, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is not set")
	}

	pub, err := mq.NewPublisher(cfg.RabbitURL, mq.QueueJobs)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}

	b := &Bot{
		cfg:    cfg,
		log:    log,
		worker: workerclient.New(cfg.WorkerURL, log),
		pub:    pub,
		sub:    mq.NewConsumer(cfg.RabbitURL, mq.QueueCompleted, "vdownloader-telegram"),
		states: make(map[int64]*userState),
		jobs:   make(map[string]*pendingJob),
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

// Run starts the completion consumer and Telegram polling.
// It blocks until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	go b.sub.Consume(ctx, b.log, b.handleCompleted)
	b.tg.Start(ctx)
}

// Close closes the RabbitMQ job-requests publisher.
func (b *Bot) Close() {
	if err := b.pub.Close(); err != nil {
		b.log.Error("close rabbitmq publisher", "err", err)
	}
}

func (b *Bot) registerHandlers() {
	b.tg.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, b.onStart)
	b.tg.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, b.onStart)
	// Step 1: "q:<height>" picks a video quality tier, "q:audio" picks the audio-only branch.
	b.tg.RegisterHandler(bot.HandlerTypeCallbackQueryData, "q:", bot.MatchTypePrefix, b.onQualityCallback)
	// Step 2 (video): "va:1" with audio, "va:0" without.
	b.tg.RegisterHandler(bot.HandlerTypeCallbackQueryData, "va:", bot.MatchTypePrefix, b.onVideoAudioCallback)
	// Step 2 (audio): "af:<format>", one of the audio_formats offered in step 1.
	b.tg.RegisterHandler(bot.HandlerTypeCallbackQueryData, "af:", bot.MatchTypePrefix, b.onAudioFormatCallback)
}
