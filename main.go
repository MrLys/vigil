package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vigil/gitea"
	"vigil/notifier"
	"vigil/processor"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Setup Gitea client
	giteaClient := setupGitea()

	// Setup notifiers
	notifiers := setupNotifiers()

	// Setup processor
	proc := setupProcessor(giteaClient, notifiers)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Start processor (blocks until context is cancelled)
	proc.Start(ctx)
	log.Println("Shutdown complete")
}

func setupGitea() *gitea.Client {
	url := os.Getenv("GITEA_URL")
	if url == "" {
		log.Fatal("GITEA_URL is required")
	}

	token := os.Getenv("GITEA_TOKEN")
	if token == "" {
		log.Fatal("GITEA_TOKEN is required")
	}

	owner := os.Getenv("GITEA_OWNER")
	if owner == "" {
		log.Fatal("GITEA_OWNER is required")
	}

	repo := os.Getenv("GITEA_REPO")
	if repo == "" {
		repo = "error-issues"
	}

	log.Printf("Gitea: %s/%s/%s", url, owner, repo)
	return gitea.NewClient(url, token, owner, repo)
}

func setupNotifiers() []notifier.Notifier {
	var notifiers []notifier.Notifier

	// Slack
	if webhookURL := os.Getenv("SLACK_WEBHOOK_URL"); webhookURL != "" {
		notifiers = append(notifiers, notifier.NewSlackNotifier(webhookURL))
		log.Println("Slack notifier enabled")
	}

	// Discord
	if webhookURL := os.Getenv("DISCORD_WEBHOOK_URL"); webhookURL != "" {
		notifiers = append(notifiers, notifier.NewDiscordNotifier(webhookURL))
		log.Println("Discord notifier enabled")
	}

	// Telegram
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if botToken != "" && chatID != "" {
		notifiers = append(notifiers, notifier.NewTelegramNotifier(botToken, chatID))
		log.Println("Telegram notifier enabled")
	}

	if len(notifiers) == 0 {
		log.Println("No notifiers configured (issues will still be created in Gitea)")
	}

	return notifiers
}

func setupProcessor(giteaClient *gitea.Client, notifiers []notifier.Notifier) *processor.Processor {
	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://loki:3100"
	}

	pollInterval := 30 * time.Second
	if interval := os.Getenv("LOKI_POLL_INTERVAL"); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil {
			pollInterval = d
		}
	}

	lookback := 5 * time.Minute
	if lb := os.Getenv("LOKI_LOOKBACK"); lb != "" {
		if d, err := time.ParseDuration(lb); err == nil {
			lookback = d
		}
	}

	cfg := processor.Config{
		LokiURL:      lokiURL,
		PollInterval: pollInterval,
		Lookback:     lookback,
	}

	return processor.NewProcessor(giteaClient, cfg, notifiers)
}
