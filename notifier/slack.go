package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackNotifier sends notifications to Slack via webhook
type SlackNotifier struct {
	webhookURL string
	httpClient *http.Client
}

// SlackMessage represents a Slack webhook message
type SlackMessage struct {
	Text        string            `json:"text,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackAttachment represents a Slack message attachment
type SlackAttachment struct {
	Color      string       `json:"color,omitempty"`
	Title      string       `json:"title,omitempty"`
	TitleLink  string       `json:"title_link,omitempty"`
	Text       string       `json:"text,omitempty"`
	Fields     []SlackField `json:"fields,omitempty"`
	Footer     string       `json:"footer,omitempty"`
	FooterIcon string       `json:"footer_icon,omitempty"`
	Ts         int64        `json:"ts,omitempty"`
}

// SlackField represents a field in a Slack attachment
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// NewSlackNotifier creates a new Slack notifier
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyNewIssue sends a notification for a new issue
func (s *SlackNotifier) NotifyNewIssue(issue *IssueInfo) error {
	msg := SlackMessage{
		Attachments: []SlackAttachment{
			{
				Color: "#ff0000", // red for new issues
				Title: fmt.Sprintf("New Issue #%d: %s", issue.Number, issue.Title),
				Fields: []SlackField{
					{Title: "Bug ID", Value: issue.BugID, Short: true},
					{Title: "Status Code", Value: fmt.Sprintf("%d", issue.StatusCode), Short: true},
					{Title: "Endpoint", Value: fmt.Sprintf("%s %s", issue.HTTPMethod, issue.Endpoint), Short: false},
				},
				Footer: "Issue Tracker → Gitea",
				Ts:     issue.FirstSeen.Unix(),
			},
		},
	}

	return s.send(msg)
}

// NotifyReopenedIssue sends a notification for a reopened issue
func (s *SlackNotifier) NotifyReopenedIssue(issue *IssueInfo) error {
	msg := SlackMessage{
		Attachments: []SlackAttachment{
			{
				Color: "#ff9900", // orange for reopened
				Title: fmt.Sprintf("Reopened Issue #%d: %s", issue.Number, issue.Title),
				Text:  fmt.Sprintf("This issue has been reopened. Total occurrences: %d", issue.Occurrences),
				Footer: "Issue Tracker → Gitea",
				Ts:     time.Now().Unix(),
			},
		},
	}

	return s.send(msg)
}

// Name returns the name of this notifier
func (s *SlackNotifier) Name() string {
	return "slack"
}

// send posts a message to the Slack webhook
func (s *SlackNotifier) send(msg SlackMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	resp, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}
