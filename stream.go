package taskforceai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type sseStream struct {
	taskID string
	ctx    context.Context
	cancel context.CancelFunc
	resp   *http.Response
	reader *bufio.Reader
}

func (c *Client) StreamTaskStatus(ctx context.Context, taskID string) (TaskStatusStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)

	url := c.baseURL + "/stream/" + taskID
	req, err := http.NewRequestWithContext(streamCtx, "GET", url, nil)
	if err != nil {
		cancel()
		return nil, err
	}

	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("stream error: status %d", resp.StatusCode)
	}

	return &sseStream{
		taskID: taskID,
		ctx:    streamCtx,
		cancel: cancel,
		resp:   resp,
		reader: bufio.NewReader(resp.Body),
	}, nil
}

func (s *sseStream) TaskID() string {
	return s.taskID
}

func (s *sseStream) Close() error {
	s.cancel()
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}

func (s *sseStream) Next() (TaskStatus, error) {
	for {
		select {
		case <-s.ctx.Done():
			return TaskStatus{}, s.ctx.Err()
		default:
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			return TaskStatus{}, err
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(line[5:])
			var status TaskStatus
			if err := json.Unmarshal([]byte(data), &status); err != nil {
				return TaskStatus{}, err
			}
			return status, nil
		}
	}
}

func (c *Client) RunTaskStream(ctx context.Context, prompt string, opts *TaskSubmissionOptions) (TaskStatusStream, error) {
	taskID, err := c.SubmitTask(ctx, prompt, opts)
	if err != nil {
		return nil, err
	}

	return c.StreamTaskStatus(ctx, taskID)
}
