package notifier

import "time"

// IssueInfo contains information about an issue for notifications
type IssueInfo struct {
	Number      int64
	Title       string
	BugID       string
	Endpoint    string
	HTTPMethod  string
	StatusCode  int
	FirstSeen   time.Time
	Occurrences int
}

// Notifier is the interface for sending notifications
type Notifier interface {
	NotifyNewIssue(issue *IssueInfo) error
	NotifyReopenedIssue(issue *IssueInfo) error
	Name() string
}

// MultiNotifier sends notifications to multiple notifiers
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier creates a notifier that sends to multiple destinations
func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

// NotifyNewIssue sends a new issue notification to all notifiers
func (m *MultiNotifier) NotifyNewIssue(issue *IssueInfo) error {
	var lastErr error
	for _, n := range m.notifiers {
		if err := n.NotifyNewIssue(issue); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// NotifyReopenedIssue sends a reopened issue notification to all notifiers
func (m *MultiNotifier) NotifyReopenedIssue(issue *IssueInfo) error {
	var lastErr error
	for _, n := range m.notifiers {
		if err := n.NotifyReopenedIssue(issue); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Name returns the name of this notifier
func (m *MultiNotifier) Name() string {
	return "multi"
}
