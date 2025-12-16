package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TelegramNotifier sends notifications to Telegram via Bot API
type TelegramNotifier struct {
	botToken   string
	chatID     string
	httpClient *http.Client
}

// TelegramMessage represents a Telegram sendMessage request
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// NewTelegramNotifier creates a new Telegram notifier
func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		botToken:   botToken,
		chatID:     chatID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyNewIssue sends a notification for a new issue
func (t *TelegramNotifier) NotifyNewIssue(issue *IssueInfo) error {
	text := fmt.Sprintf(
		"🔴 *New Issue \\#%d*\n\n"+
			"*Title:* %s\n"+
			"*Bug ID:* `%s`\n"+
			"*Status Code:* %d\n"+
			"*Endpoint:* `%s %s`\n"+
			"*Time:* %s",
		issue.Number,
		escapeMarkdown(issue.Title),
		issue.BugID,
		issue.StatusCode,
		issue.HTTPMethod,
		escapeMarkdown(issue.Endpoint),
		issue.FirstSeen.Format(time.RFC3339),
	)

	return t.send(text)
}

// NotifyReopenedIssue sends a notification for a reopened issue
func (t *TelegramNotifier) NotifyReopenedIssue(issue *IssueInfo) error {
	text := fmt.Sprintf(
		"🟠 *Reopened Issue \\#%d*\n\n"+
			"*Title:* %s\n"+
			"*Occurrences:* %d",
		issue.Number,
		escapeMarkdown(issue.Title),
		issue.Occurrences,
	)

	return t.send(text)
}

// Name returns the name of this notifier
func (t *TelegramNotifier) Name() string {
	return "telegram"
}

// send posts a message to Telegram
func (t *TelegramNotifier) send(text string) error {
	msg := TelegramMessage{
		ChatID:    t.chatID,
		Text:      text,
		ParseMode: "MarkdownV2",
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram message: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	resp, err := t.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send Telegram notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram API returned status %d", resp.StatusCode)
	}

	return nil
}

// escapeMarkdown escapes special characters for Telegram MarkdownV2
func escapeMarkdown(text string) string {
	chars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := text
	for _, char := range chars {
		result = escapeChar(result, char)
	}
	return result
}

func escapeChar(text, char string) string {
	var result []byte
	for i := 0; i < len(text); i++ {
		if string(text[i]) == char {
			result = append(result, '\\')
		}
		result = append(result, text[i])
	}
	return string(result)
}
