package taskforceai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultBaseURL      = "https://taskforceai.chat/api/developer"
	DefaultTimeout      = 30 * time.Second
	DefaultPollInterval = 1 * time.Second
	DefaultMaxPoll      = 60
)

type Client struct {
	apiKey       string
	baseURL      string
	timeout      time.Duration
	responseHook func(statusCode int, header map[string][]string)
	mockMode     bool
	httpClient   *http.Client
}

func NewClient(opts TaskForceAIOptions) (*Client) {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &Client{
		apiKey:       opts.APIKey,
		baseURL:      baseURL,
		timeout:      timeout,
		responseHook: opts.ResponseHook,
		mockMode:     opts.MockMode,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	url := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-SDK-Language", "go")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if c.responseHook != nil {
		c.responseHook(resp.StatusCode, resp.Header)
	}

	return resp, nil
}

func (c *Client) SubmitTask(ctx context.Context, prompt string, opts *TaskSubmissionOptions) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	body := map[string]interface{}{
		"prompt": prompt,
	}
	if opts != nil {
		body["options"] = opts
	}

	resp, err := c.doRequest(ctx, "POST", "/run", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("failed to submit task: status %d", resp.StatusCode)
	}

	var result struct {
		TaskID string `json:"taskId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.TaskID, nil
}

func (c *Client) GetTaskStatus(ctx context.Context, taskID string) (TaskStatus, error) {
	resp, err := c.doRequest(ctx, "GET", "/status/"+taskID, nil)
	if err != nil {
		return TaskStatus{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TaskStatus{}, fmt.Errorf("failed to get task status: status %d", resp.StatusCode)
	}

	var status TaskStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return TaskStatus{}, err
	}

	return status, nil
}

func (c *Client) WaitForCompletion(ctx context.Context, taskID string, pollInterval time.Duration, maxAttempts int, callback TaskStatusCallback) (TaskStatus, error) {
	if pollInterval == 0 {
		pollInterval = DefaultPollInterval
	}
	if maxAttempts == 0 {
		maxAttempts = DefaultMaxPoll
	}

	for i := 0; i < maxAttempts; i++ {
		status, err := c.GetTaskStatus(ctx, taskID)
		if err != nil {
			return TaskStatus{}, err
		}

		if callback != nil {
			callback(status)
		}

		if status.Status == "completed" {
			return status, nil
		}
		if status.Status == "failed" {
			errMsg := "task failed"
			if status.Error != nil {
				errMsg = *status.Error
			}
			return status, fmt.Errorf("task failed: %s", errMsg)
		}

		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return TaskStatus{}, fmt.Errorf("task timed out")
}

func (c *Client) RunTask(ctx context.Context, prompt string, opts *TaskSubmissionOptions, pollInterval time.Duration, maxAttempts int, callback TaskStatusCallback) (TaskStatus, error) {
	taskID, err := c.SubmitTask(ctx, prompt, opts)
	if err != nil {
		return TaskStatus{}, err
	}

	return c.WaitForCompletion(ctx, taskID, pollInterval, maxAttempts, callback)
}
