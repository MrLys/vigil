package processor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"vigil/gitea"
	"vigil/loki"
	"vigil/notifier"
)

// Processor handles log processing and issue creation in Gitea
type Processor struct {
	giteaClient  *gitea.Client
	lokiClient   *loki.Client
	notifiers    []notifier.Notifier
	pollInterval time.Duration
	lookback     time.Duration
	lastPoll     time.Time
}

// Config holds processor configuration
type Config struct {
	LokiURL      string
	PollInterval time.Duration
	Lookback     time.Duration
}

// NewProcessor creates a new log processor
func NewProcessor(giteaClient *gitea.Client, cfg Config, notifiers []notifier.Notifier) *Processor {
	return &Processor{
		giteaClient:  giteaClient,
		lokiClient:   loki.NewClient(cfg.LokiURL),
		notifiers:    notifiers,
		pollInterval: cfg.PollInterval,
		lookback:     cfg.Lookback,
		lastPoll:     time.Now().Add(-cfg.Lookback),
	}
}

// Start begins the log polling loop
func (p *Processor) Start(ctx context.Context) {
	log.Printf("Starting log processor (poll interval: %s, lookback: %s)", p.pollInterval, p.lookback)

	// Test Gitea connection
	if err := p.giteaClient.TestConnection(); err != nil {
		log.Printf("WARNING: Gitea connection test failed: %v", err)
		log.Println("Will retry on first poll...")
	} else {
		log.Println("Gitea connection successful")
		// Ensure required labels exist
		p.ensureLabels()
	}

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	// Initial poll
	p.poll()

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping log processor")
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

// ensureLabels creates required labels if they don't exist
func (p *Processor) ensureLabels() {
	labels := map[string]string{
		"auto-generated":    "808080", // gray
		"severity:critical": "ff0000", // red
		"severity:error":    "ff9900", // orange
	}

	for name, color := range labels {
		if err := p.giteaClient.EnsureLabel(name, color); err != nil {
			log.Printf("Warning: failed to ensure label %s: %v", name, err)
		}
	}
}

// poll queries Loki for new error logs
func (p *Processor) poll() {
	now := time.Now()
	start := p.lastPoll

	// Query for error logs - use line filter first (more reliable), then parse JSON
	// The Go code will do final filtering via IsError()
	query := `{job=~".+"} |~ "ERROR|\"status\":5[0-9]{2}" | json`

	entries, err := p.lokiClient.QueryRange(query, start, now, 1000)
	if err != nil {
		log.Printf("Error querying Loki: %v", err)
		return
	}

	p.lastPoll = now

	if len(entries) == 0 {
		log.Printf("No entries found from Loki query")
		return
	}

	log.Printf("Found %d entries from Loki, filtering for errors...", len(entries))

	errorCount := 0
	for _, entry := range entries {
		if entry.IsError() {
			errorCount++
			log.Printf("Processing error: level=%s status=%d msg=%s", entry.Level, entry.Status, entry.Message)
			if err := p.processEntry(entry); err != nil {
				log.Printf("Error processing log entry: %v", err)
			}
		}
	}

	if errorCount > 0 {
		log.Printf("Processed %d error entries", errorCount)
	}
}

// processEntry processes a single log entry
func (p *Processor) processEntry(entry loki.LogEntry) error {
	bugID := GenerateBugID(entry)
	bugIDLabel := fmt.Sprintf("bugid:%s", bugID)

	// Search for existing issue with this bugId
	issues, err := p.giteaClient.SearchIssues(bugIDLabel)
	if err != nil {
		return fmt.Errorf("failed to search issues: %w", err)
	}

	if len(issues) == 0 {
		// New issue - create it
		return p.createNewIssue(entry, bugID, bugIDLabel)
	}

	// Existing issue - add comment and potentially reopen
	existing := issues[0]
	return p.updateExistingIssue(existing, entry)
}

// createNewIssue creates a new issue in Gitea
func (p *Processor) createNewIssue(entry loki.LogEntry, bugID, bugIDLabel string) error {
	title := generateTitle(entry)
	body := generateBody(entry, bugID)

	// Determine labels
	labels := []string{"auto-generated", bugIDLabel}
	if entry.Status >= 500 {
		labels = append(labels, "severity:critical")
	} else {
		labels = append(labels, "severity:error")
	}

	// Ensure bugid label exists
	if err := p.giteaClient.EnsureLabel(bugIDLabel, "0366d6"); err != nil { // blue
		log.Printf("Warning: failed to create bugid label: %v", err)
	}

	issue, err := p.giteaClient.CreateIssue(title, body, labels)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}

	log.Printf("Created new issue #%d: %s (bugId: %s)", issue.Number, title, bugID)

	// Send notifications
	for _, n := range p.notifiers {
		if err := n.NotifyNewIssue(&notifier.IssueInfo{
			Number:     issue.Number,
			Title:      title,
			BugID:      bugID,
			Endpoint:   entry.Action,
			HTTPMethod: entry.Method,
			StatusCode: entry.Status,
			FirstSeen:  entry.Timestamp,
		}); err != nil {
			log.Printf("Error sending notification: %v", err)
		}
	}

	return nil
}

// updateExistingIssue adds a comment to an existing issue and reopens if closed
func (p *Processor) updateExistingIssue(existing gitea.Issue, entry loki.LogEntry) error {
	// Get occurrence count (comments + 1 for original)
	occurrences := existing.Comments + 2 // +1 for original, +1 for this occurrence

	// Add comment
	comment := generateComment(entry, occurrences)
	if err := p.giteaClient.AddComment(existing.Number, comment); err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	// Reopen if closed
	if existing.State == "closed" {
		if err := p.giteaClient.ReopenIssue(existing.Number); err != nil {
			log.Printf("Warning: failed to reopen issue #%d: %v", existing.Number, err)
		} else {
			log.Printf("Reopened issue #%d", existing.Number)

			// Notify about reopened issue
			for _, n := range p.notifiers {
				if err := n.NotifyReopenedIssue(&notifier.IssueInfo{
					Number:      existing.Number,
					Title:       existing.Title,
					Occurrences: occurrences,
				}); err != nil {
					log.Printf("Error sending notification: %v", err)
				}
			}
		}
	}

	log.Printf("Updated issue #%d (occurrence #%d)", existing.Number, occurrences)
	return nil
}

// GenerateBugID creates a unique bug ID from log entry
func GenerateBugID(entry loki.LogEntry) string {
	// If explicit bugId is provided in the log, use it
	if entry.BugID != "" {
		return entry.BugID
	}

	// Auto-generate from log fields
	endpoint := normalizeEndpoint(entry.Action)

	data := fmt.Sprintf("%s|%s|%d|%s",
		entry.Method,
		endpoint,
		entry.Status,
		entry.Source.Function,
	)

	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8]) // Shorter for readability
}

// normalizeEndpoint replaces dynamic path segments with placeholders
func normalizeEndpoint(endpoint string) string {
	// Replace numeric IDs
	numericID := regexp.MustCompile(`/\d+`)
	endpoint = numericID.ReplaceAllString(endpoint, "/:id")

	// Replace UUIDs
	uuid := regexp.MustCompile(`/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	endpoint = uuid.ReplaceAllString(endpoint, "/:uuid")

	return endpoint
}

// generateTitle creates a title for the issue
func generateTitle(entry loki.LogEntry) string {
	var parts []string

	if entry.Status >= 500 {
		parts = append(parts, fmt.Sprintf("[%d]", entry.Status))
	} else if entry.Level != "" {
		parts = append(parts, fmt.Sprintf("[%s]", strings.ToUpper(entry.Level)))
	}

	if entry.Method != "" && entry.Action != "" {
		parts = append(parts, fmt.Sprintf("%s %s", entry.Method, normalizeEndpoint(entry.Action)))
	}

	if entry.Message != "" && len(entry.Message) < 80 {
		parts = append(parts, entry.Message)
	}

	if len(parts) == 0 {
		return "Unknown error"
	}

	return strings.Join(parts, " - ")
}

// generateBody creates the issue body in Markdown
func generateBody(entry loki.LogEntry, bugID string) string {
	var sb strings.Builder

	sb.WriteString("## Error Details\n\n")

	if entry.Message != "" {
		sb.WriteString(fmt.Sprintf("**Message:** %s\n\n", entry.Message))
	}

	if entry.Source.Function != "" {
		sb.WriteString(fmt.Sprintf("**Source:** `%s`\n", entry.Source.Function))
	}

	if entry.Source.File != "" {
		sb.WriteString(fmt.Sprintf("**File:** `%s:%d`\n", entry.Source.File, entry.Source.Line))
	}

	sb.WriteString("\n## Request Info\n\n")

	if entry.Method != "" {
		sb.WriteString(fmt.Sprintf("- **Method:** %s\n", entry.Method))
	}
	if entry.Action != "" {
		sb.WriteString(fmt.Sprintf("- **Endpoint:** %s\n", entry.Action))
	}
	if entry.Status > 0 {
		sb.WriteString(fmt.Sprintf("- **Status Code:** %d\n", entry.Status))
	}
	if entry.RequestID != "" {
		sb.WriteString(fmt.Sprintf("- **Request ID:** `%s`\n", entry.RequestID))
	}
	if entry.TraceID != "" {
		sb.WriteString(fmt.Sprintf("- **Trace ID:** `%s`\n", entry.TraceID))
	}
	if entry.UserID != "" {
		sb.WriteString(fmt.Sprintf("- **User ID:** %s\n", entry.UserID))
	}

	sb.WriteString("\n## Sample Log\n\n```json\n")
	if jsonBytes, err := json.MarshalIndent(entry.Parsed, "", "  "); err == nil {
		sb.Write(jsonBytes)
	}
	sb.WriteString("\n```\n")

	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("*Bug ID: `%s`*\n", bugID))
	sb.WriteString("*Auto-generated by issue-tracker*\n")

	return sb.String()
}

// generateComment creates a comment for duplicate occurrences
func generateComment(entry loki.LogEntry, occurrences int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("**Occurred again** at `%s`\n\n", entry.Timestamp.Format(time.RFC3339)))

	if entry.RequestID != "" {
		sb.WriteString(fmt.Sprintf("- Request ID: `%s`\n", entry.RequestID))
	}
	if entry.TraceID != "" {
		sb.WriteString(fmt.Sprintf("- Trace ID: `%s`\n", entry.TraceID))
	}
	if entry.UserID != "" {
		sb.WriteString(fmt.Sprintf("- User ID: %s\n", entry.UserID))
	}

	sb.WriteString(fmt.Sprintf("- Total occurrences: **%d**\n", occurrences))

	return sb.String()
}
