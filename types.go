package taskforceai

import (
	"time"
)

// TaskForceAIOptions defines configuration for the TaskForceAI client.
type TaskForceAIOptions struct {
	APIKey       string
	BaseURL      string
	Timeout      time.Duration
	ResponseHook func(statusCode int, header map[string][]string)
	MockMode     bool
}

// TaskSubmissionOptions defines parameters for submitting a task.
type TaskSubmissionOptions struct {
	ModelID     string                 `json:"modelId,omitempty"`
	Silent      bool                   `json:"silent,omitempty"`
	Mock        bool                   `json:"mock,omitempty"`
	VercelAIKey string                 `json:"vercelAiKey,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TaskStatus represents the current state of a task.
type TaskStatus struct {
	TaskID   string                 `json:"taskId"`
	Status   string                 `json:"status"` // "processing", "completed", "failed"
	Result   *string                `json:"result,omitempty"`
	Error    *string                `json:"error,omitempty"`
	Warnings []string               `json:"warnings,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TaskResult is a completed TaskStatus.
type TaskResult struct {
	TaskStatus
}

// TaskStatusCallback is called during polling or streaming.
type TaskStatusCallback func(status TaskStatus)

// TaskStatusStream provides an interface for consuming task events.
type TaskStatusStream interface {
	Next() (TaskStatus, error)
	Close() error
	TaskID() string
}
