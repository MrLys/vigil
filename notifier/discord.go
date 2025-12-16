package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DiscordNotifier sends notifications to Discord via webhook
type DiscordNotifier struct {
	webhookURL string
	httpClient *http.Client
}

// DiscordMessage represents a Discord webhook message
type DiscordMessage struct {
	Content string         `json:"content,omitempty"`
	Embeds  []DiscordEmbed `json:"embeds,omitempty"`
}

// DiscordEmbed represents a Discord embed
type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	URL         string              `json:"url,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Footer      *DiscordEmbedFooter `json:"footer,omitempty"`
}

// DiscordEmbedField represents a field in a Discord embed
type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// DiscordEmbedFooter represents a footer in a Discord embed
type DiscordEmbedFooter struct {
	Text    string `json:"text"`
	IconURL string `json:"icon_url,omitempty"`
}

// NewDiscordNotifier creates a new Discord notifier
func NewDiscordNotifier(webhookURL string) *DiscordNotifier {
	return &DiscordNotifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyNewIssue sends a notification for a new issue
func (d *DiscordNotifier) NotifyNewIssue(issue *IssueInfo) error {
	msg := DiscordMessage{
		Embeds: []DiscordEmbed{
			{
				Title:     fmt.Sprintf("New Issue #%d: %s", issue.Number, issue.Title),
				Color:     0xff0000, // red
				Timestamp: issue.FirstSeen.Format(time.RFC3339),
				Fields: []DiscordEmbedField{
					{Name: "Bug ID", Value: issue.BugID, Inline: true},
					{Name: "Status Code", Value: fmt.Sprintf("%d", issue.StatusCode), Inline: true},
					{Name: "Endpoint", Value: fmt.Sprintf("%s %s", issue.HTTPMethod, issue.Endpoint), Inline: false},
				},
				Footer: &DiscordEmbedFooter{
					Text: "Issue Tracker → Gitea",
				},
			},
		},
	}

	return d.send(msg)
}

// NotifyReopenedIssue sends a notification for a reopened issue
func (d *DiscordNotifier) NotifyReopenedIssue(issue *IssueInfo) error {
	msg := DiscordMessage{
		Embeds: []DiscordEmbed{
			{
				Title:       fmt.Sprintf("Reopened Issue #%d: %s", issue.Number, issue.Title),
				Description: fmt.Sprintf("This issue has been reopened. Total occurrences: %d", issue.Occurrences),
				Color:       0xff9900, // orange
				Timestamp:   time.Now().Format(time.RFC3339),
				Footer: &DiscordEmbedFooter{
					Text: "Issue Tracker → Gitea",
				},
			},
		},
	}

	return d.send(msg)
}

// Name returns the name of this notifier
func (d *DiscordNotifier) Name() string {
	return "discord"
}

// send posts a message to the Discord webhook
func (d *DiscordNotifier) send(msg DiscordMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord message: %w", err)
	}

	resp, err := d.httpClient.Post(d.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send Discord notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}
