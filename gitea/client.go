package gitea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a Gitea API client
type Client struct {
	baseURL    string
	token      string
	owner      string
	repo       string
	httpClient *http.Client
}

// NewClient creates a new Gitea client
func NewClient(baseURL, token, owner, repo string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		owner:   owner,
		repo:    repo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Issue represents a Gitea issue
type Issue struct {
	ID        int64     `json:"id"`
	Number    int64     `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []Label   `json:"labels"`
	Comments  int       `json:"comments"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Label represents a Gitea label
type Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CreateIssueRequest is the request body for creating an issue
type CreateIssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels,omitempty"`
}

// CreateCommentRequest is the request body for creating a comment
type CreateCommentRequest struct {
	Body string `json:"body"`
}

// UpdateIssueRequest is the request body for updating an issue
type UpdateIssueRequest struct {
	State string `json:"state,omitempty"`
}

// CreateLabelRequest is the request body for creating a label
type CreateLabelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// SearchIssues searches for issues by label
func (c *Client) SearchIssues(labelName string) ([]Issue, error) {
	params := url.Values{}
	params.Set("labels", labelName)
	params.Set("state", "all") // Include closed issues for deduplication

	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues?%s", c.baseURL, c.owner, c.repo, params.Encode())

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search issues: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitea returned status %d: %s", resp.StatusCode, string(body))
	}

	var issues []Issue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return issues, nil
}

// CreateIssue creates a new issue
func (c *Client) CreateIssue(title, body string, labels []string) (*Issue, error) {
	reqBody := CreateIssueRequest{
		Title:  title,
		Body:   body,
		Labels: labels,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues", c.baseURL, c.owner, c.repo)

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitea returned status %d: %s", resp.StatusCode, string(body))
	}

	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &issue, nil
}

// AddComment adds a comment to an issue
func (c *Client) AddComment(issueNumber int64, body string) error {
	reqBody := CreateCommentRequest{Body: body}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments", c.baseURL, c.owner, c.repo, issueNumber)

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ReopenIssue reopens a closed issue
func (c *Client) ReopenIssue(issueNumber int64) error {
	reqBody := UpdateIssueRequest{State: "open"}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d", c.baseURL, c.owner, c.repo, issueNumber)

	req, err := http.NewRequest("PATCH", reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reopen issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// EnsureLabel ensures a label exists, creating it if necessary
func (c *Client) EnsureLabel(name, color string) error {
	// Just try to create - Gitea returns 409 if it already exists
	err := c.createLabel(name, color)
	if err != nil && !strings.Contains(err.Error(), "409") {
		return err
	}
	return nil
}

// createLabel creates a new label
func (c *Client) createLabel(name, color string) error {
	reqBody := CreateLabelRequest{
		Name:  name,
		Color: color,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/labels", c.baseURL, c.owner, c.repo)

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create label: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetIssueCommentCount returns the number of comments on an issue
func (c *Client) GetIssueCommentCount(issueNumber int64) (int, error) {
	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d", c.baseURL, c.owner, c.repo, issueNumber)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return 0, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get issue: %w", err)
	}
	defer resp.Body.Close()

	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return 0, fmt.Errorf("failed to decode issue: %w", err)
	}

	return issue.Comments, nil
}

// setAuth sets the authorization header
func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "token "+c.token)
}

// TestConnection tests the connection to Gitea
func (c *Client) TestConnection() error {
	reqURL := fmt.Sprintf("%s/api/v1/repos/%s/%s", c.baseURL, c.owner, c.repo)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Gitea: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("repository %s/%s not found", c.owner, c.repo)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid Gitea token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gitea returned status %d", resp.StatusCode)
	}

	return nil
}
