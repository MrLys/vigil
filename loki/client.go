package loki

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a Loki API client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Loki client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// QueryResponse represents the Loki query response
type QueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string   `json:"resultType"`
		Result     []Stream `json:"result"`
	} `json:"data"`
}

// Stream represents a log stream from Loki
type Stream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"` // [timestamp_ns, log_line]
}

// LogEntry represents a parsed log entry
type LogEntry struct {
	Timestamp time.Time
	Raw       string
	Parsed    map[string]interface{}

	// Common fields extracted from logs
	Level      string
	Message    string
	Method     string
	Action     string // endpoint/path
	Status     int
	RequestID  string
	TraceID    string
	UserID     string
	BugID      string // explicit bug ID if provided in logs
	Source     SourceInfo
	ElapsedMs  float64
}

// SourceInfo contains information about the log source
type SourceInfo struct {
	Function string
	File     string
	Line     int
}

// QueryRange queries Loki for logs within a time range
func (c *Client) QueryRange(query string, start, end time.Time, limit int) ([]LogEntry, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", fmt.Sprintf("%d", start.UnixNano()))
	params.Set("end", fmt.Sprintf("%d", end.UnixNano()))
	params.Set("limit", fmt.Sprintf("%d", limit))

	reqURL := fmt.Sprintf("%s/loki/api/v1/query_range?%s", c.baseURL, params.Encode())

	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query Loki: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Loki returned status %d: %s", resp.StatusCode, string(body))
	}

	var queryResp QueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&queryResp); err != nil {
		return nil, fmt.Errorf("failed to decode Loki response: %w", err)
	}

	return parseStreams(queryResp.Data.Result), nil
}

// parseStreams converts Loki streams to LogEntry slices
func parseStreams(streams []Stream) []LogEntry {
	var entries []LogEntry

	for _, stream := range streams {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}

			// Parse timestamp (nanoseconds)
			var ts time.Time
			var tsNano int64
			if _, err := fmt.Sscanf(value[0], "%d", &tsNano); err == nil {
				ts = time.Unix(0, tsNano)
			}

			entry := LogEntry{
				Timestamp: ts,
				Raw:       value[1],
				Parsed:    make(map[string]interface{}),
			}

			// Try to parse JSON log
			if err := json.Unmarshal([]byte(value[1]), &entry.Parsed); err == nil {
				extractFields(&entry)
			}

			entries = append(entries, entry)
		}
	}

	return entries
}

// extractFields extracts common fields from parsed JSON log
func extractFields(entry *LogEntry) {
	if level, ok := entry.Parsed["level"].(string); ok {
		entry.Level = level
	}
	if msg, ok := entry.Parsed["msg"].(string); ok {
		entry.Message = msg
	}
	if method, ok := entry.Parsed["method"].(string); ok {
		entry.Method = method
	}
	if action, ok := entry.Parsed["action"].(string); ok {
		entry.Action = action
	}
	if status, ok := entry.Parsed["status"].(float64); ok {
		entry.Status = int(status)
	}
	if requestID, ok := entry.Parsed["requestId"].(string); ok {
		entry.RequestID = requestID
	}
	if traceID, ok := entry.Parsed["traceId"].(string); ok {
		entry.TraceID = traceID
	}
	if userID, ok := entry.Parsed["userid"].(string); ok {
		entry.UserID = userID
	}
	if bugID, ok := entry.Parsed["bugId"].(string); ok {
		entry.BugID = bugID
	}
	if elapsed, ok := entry.Parsed["elapsed_ms"].(float64); ok {
		entry.ElapsedMs = elapsed
	}

	// Extract source info
	if source, ok := entry.Parsed["source"].(map[string]interface{}); ok {
		if fn, ok := source["function"].(string); ok {
			entry.Source.Function = fn
		}
		if file, ok := source["file"].(string); ok {
			entry.Source.File = file
		}
		if line, ok := source["line"].(float64); ok {
			entry.Source.Line = int(line)
		}
	}
}

// IsError returns true if this log entry represents an error
func (e *LogEntry) IsError() bool {
	if e.Level == "ERROR" || e.Level == "error" {
		return true
	}
	if e.Status >= 500 {
		return true
	}
	return false
}
