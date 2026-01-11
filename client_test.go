package taskforceai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_SubmitTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/run" {
			t.Errorf("expected path /run, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"taskId": "test-task-123"}`))
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	taskID, err := client.SubmitTask(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if taskID != "test-task-123" {
		t.Errorf("expected task ID test-task-123, got %s", taskID)
	}
}

func TestClient_WaitForCompletion(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
		if attempts < 2 {
			_, _ = w.Write([]byte(`{"taskId": "task-1", "status": "processing"}`))
		} else {
			_, _ = w.Write([]byte(`{"taskId": "task-1", "status": "completed", "result": "done"}`))
		}
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{
		BaseURL: server.URL,
	})

	status, err := client.WaitForCompletion(context.Background(), "task-1", 1*time.Millisecond, 5, nil)
	if err != nil {
		t.Fatalf("WaitForCompletion failed: %v", err)
	}

	if status.Status != "completed" {
		t.Errorf("expected status completed, got %s", status.Status)
	}
	if status.Result == nil || *status.Result != "done" {
		t.Errorf("expected result done, got %v", status.Result)
	}
}
