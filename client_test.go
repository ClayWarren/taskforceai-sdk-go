package taskforceai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient_Defaults(t *testing.T) {
	client := NewClient(TaskForceAIOptions{})
	if client.baseURL != DefaultBaseURL {
		t.Errorf("expected default base URL, got %s", client.baseURL)
	}
	if client.timeout != DefaultTimeout {
		t.Errorf("expected default timeout, got %v", client.timeout)
	}
}

func TestClient_doRequest_Errors(t *testing.T) {
	// 1. Marshaling error
	client := NewClient(TaskForceAIOptions{})
	_, err := client.doRequest(context.Background(), "POST", "/", make(chan int))
	if err == nil {
		t.Error("expected marshal error for chan type, got nil")
	}

	// 2. NewRequest error (invalid method)
	_, err = client.doRequest(context.Background(), "INVALID METHOD", "/", nil)
	if err == nil {
		t.Error("expected error for invalid HTTP method, got nil")
	}

	// 3. Client.Do error (invalid URL/network error)
	client = NewClient(TaskForceAIOptions{BaseURL: "http://invalid-domain-that-does-not-exist.test"})
	_, err = client.doRequest(context.Background(), "GET", "/", nil)
	if err == nil {
		t.Error("expected network error, got nil")
	}
}

func TestClient_doRequest_AuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Errorf("expected auth header, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{
		BaseURL: server.URL,
		APIKey:  "secret-key",
	})
	_, _ = client.doRequest(context.Background(), "GET", "/", nil)
}

func TestClient_doRequest_Hook(t *testing.T) {
	hookCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{
		BaseURL: server.URL,
		ResponseHook: func(statusCode int, header map[string][]string) {
			hookCalled = true
			if statusCode != http.StatusCreated {
				t.Errorf("expected status 201 in hook, got %d", statusCode)
			}
		},
	})

	_, _ = client.doRequest(context.Background(), "GET", "/", nil)
	if !hookCalled {
		t.Error("expected response hook to be called")
	}
}

func TestClient_SubmitTask_WithOpts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"taskId": "task-with-opts"}`))
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{BaseURL: server.URL})
	_, err := client.SubmitTask(context.Background(), "hello", &TaskSubmissionOptions{ModelID: "test-model"})
	if err != nil {
		t.Errorf("SubmitTask with opts failed: %v", err)
	}
}

func TestClient_SubmitTask_Errors(t *testing.T) {
	client := NewClient(TaskForceAIOptions{})
	
	// 1. Prompt required
	_, err := client.SubmitTask(context.Background(), "", nil)
	if err == nil || err.Error() != "prompt is required" {
		t.Errorf("expected prompt required error, got %v", err)
	}

	// 2. Server 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client = NewClient(TaskForceAIOptions{BaseURL: server.URL})
	_, err = client.SubmitTask(context.Background(), "hello", nil)
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Errorf("expected 500 error, got %v", err)
	}

	// 3. Malformed JSON response
	malformedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{malformed}`))
	}))
	defer malformedServer.Close()

	client = NewClient(TaskForceAIOptions{BaseURL: malformedServer.URL})
	_, err = client.SubmitTask(context.Background(), "hello", nil)
	if err == nil {
		t.Error("expected JSON decode error, got nil")
	}
}

func TestClient_GetTaskStatus_Errors(t *testing.T) {
	// 1. 404 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{BaseURL: server.URL})
	_, err := client.GetTaskStatus(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "status 404") {
		t.Errorf("expected 404 error, got %v", err)
	}

	// 2. Malformed JSON
	malformedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{malformed}`))
	}))
	defer malformedServer.Close()

	client = NewClient(TaskForceAIOptions{BaseURL: malformedServer.URL})
	_, err = client.GetTaskStatus(context.Background(), "id")
	if err == nil {
		t.Error("expected JSON decode error, got nil")
	}
}

func TestClient_RunTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/run" {
			_, _ = w.Write([]byte(`{"taskId": "task-run-1"}`))
		} else {
			_, _ = w.Write([]byte(`{"taskId": "task-run-1", "status": "completed", "result": "run-done"}`))
		}
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{BaseURL: server.URL})
	status, err := client.RunTask(context.Background(), "run me", nil, 1*time.Millisecond, 2, nil)
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if status.TaskID != "task-run-1" || *status.Result != "run-done" {
		t.Errorf("unexpected RunTask result: %+v", status)
	}

	// Test callback
	callbackCalled := false
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"taskId": "task-cb", "status": "completed", "result": "cb-done"}`))
	}))
	defer server.Close()
	client = NewClient(TaskForceAIOptions{BaseURL: server.URL})
	_, _ = client.WaitForCompletion(context.Background(), "task-cb", 1*time.Millisecond, 1, func(status TaskStatus) {
		callbackCalled = true
	})
	if !callbackCalled {
		t.Error("expected callback to be called")
	}

	// Test RunTask submission error
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer errServer.Close()
	client = NewClient(TaskForceAIOptions{BaseURL: errServer.URL})
	_, err = client.RunTask(context.Background(), "run me", nil, 0, 0, nil)
	if err == nil {
		t.Error("expected RunTask submission error, got nil")
	}
}

func TestClient_WaitForCompletion_Failures(t *testing.T) {
	// 1. Task failure path
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"taskId": "task-fail", "status": "failed", "error": "test failure"}`))
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{BaseURL: server.URL})
	_, err := client.WaitForCompletion(context.Background(), "task-fail", 1*time.Millisecond, 1, nil)
	if err == nil || !strings.Contains(err.Error(), "test failure") {
		t.Errorf("expected task failure error, got %v", err)
	}

	// 2. Timeout path
	timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"taskId": "task-timeout", "status": "processing"}`))
	}))
	defer timeoutServer.Close()

	client = NewClient(TaskForceAIOptions{BaseURL: timeoutServer.URL})
	_, err = client.WaitForCompletion(context.Background(), "task-timeout", 1*time.Millisecond, 1, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got %v", err)
	}

	// 3. Context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.WaitForCompletion(ctx, "task-cancel", 1*time.Millisecond, 5, nil)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context cancelled error, got %v", err)
	}

		// 4. Polling error (e.g. 500 during poll)

		pollErrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			w.WriteHeader(http.StatusInternalServerError)

		}))

		defer pollErrServer.Close()

		client = NewClient(TaskForceAIOptions{BaseURL: pollErrServer.URL})

		_, err = client.WaitForCompletion(context.Background(), "id", 0, 0, nil) // use defaults

		if err == nil {

			t.Error("expected polling error, got nil")

		}

	

		// 5. Context Done in select

		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			w.WriteHeader(http.StatusOK)

			_, _ = w.Write([]byte(`{"taskId": "id", "status": "processing"}`))

		}))

		defer server.Close()

		client = NewClient(TaskForceAIOptions{BaseURL: server.URL})

		ctx, cancel = context.WithCancel(context.Background())

		go func() {

			time.Sleep(10 * time.Millisecond)

			cancel()

		}()

		_, err = client.WaitForCompletion(ctx, "id", 50*time.Millisecond, 2, nil)

		if err == nil || !strings.Contains(err.Error(), "context canceled") {

			t.Errorf("expected context canceled in select, got %v", err)

		}

	}

func TestClient_StreamTaskStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/run" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"taskId": "task-stream-1"}`))
			return
		}

		if r.Header.Get("Authorization") != "Bearer stream-key" {
			t.Errorf("expected auth header in stream, got %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"taskId\": \"task-stream-1\", \"status\": \"processing\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"taskId\": \"task-stream-1\", \"status\": \"completed\", \"result\": \"streamed\"}\n\n"))
	}))
	defer server.Close()

	client := NewClient(TaskForceAIOptions{BaseURL: server.URL, APIKey: "stream-key"})
	
	// Test RunTaskStream
	stream, err := client.RunTaskStream(context.Background(), "stream me", nil)
	if err != nil {
		t.Fatalf("RunTaskStream failed: %v", err)
	}
	defer stream.Close()

	if stream.TaskID() != "task-stream-1" {
		t.Errorf("expected task ID task-stream-1, got %s", stream.TaskID())
	}

	// First event
	ev1, err := stream.Next()
	if err != nil {
		t.Fatalf("Next ev1 failed: %v", err)
	}
	if ev1.Status != "processing" {
		t.Errorf("expected status processing, got %s", ev1.Status)
	}

	// Second event
	ev2, err := stream.Next()
	if err != nil {
		t.Fatalf("Next ev2 failed: %v", err)
	}
	if ev2.Status != "completed" || *ev2.Result != "streamed" {
		t.Errorf("unexpected status/result: %+v", ev2)
	}

	// Close
	if err := stream.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestClient_StreamTaskStatus_Errors(t *testing.T) {
	// 1. Submission error for RunTaskStream
	client := NewClient(TaskForceAIOptions{BaseURL: "http://invalid"})
	_, err := client.RunTaskStream(context.Background(), "prompt", nil)
	if err == nil {
		t.Error("expected RunTaskStream submission error, got nil")
	}

	// 2. NewRequest error
	client = NewClient(TaskForceAIOptions{BaseURL: " :invalid-url"})
	_, err = client.StreamTaskStatus(context.Background(), "id")
	if err == nil {
		t.Error("expected StreamTaskStatus request error, got nil")
	}

	// 3. Client.Do error
	client = NewClient(TaskForceAIOptions{BaseURL: "http://non-existent-domain.test"})
	_, err = client.StreamTaskStatus(context.Background(), "id")
	if err == nil {
		t.Error("expected network error, got nil")
	}

	// 4. Non-200 status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	client = NewClient(TaskForceAIOptions{BaseURL: server.URL})
	_, err = client.StreamTaskStatus(context.Background(), "task-forbidden")
	if err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Errorf("expected 403 error, got %v", err)
	}

	// 5. Malformed JSON in stream
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {malformed}\n\n"))
	}))
	defer jsonServer.Close()
	client = NewClient(TaskForceAIOptions{BaseURL: jsonServer.URL})
	stream, _ := client.StreamTaskStatus(context.Background(), "task-malformed")
	_, err = stream.Next()
	if err == nil {
		t.Error("expected JSON unmarshal error, got nil")
	}
	_ = stream.Close()

	// 6. Reader error (connection closed)
	readerErrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Close immediately
	}))
	defer readerErrServer.Close()
	client = NewClient(TaskForceAIOptions{BaseURL: readerErrServer.URL})
	stream, _ = client.StreamTaskStatus(context.Background(), "id")
	_, err = stream.Next()
	if err == nil {
		t.Error("expected EOF or read error, got nil")
	}
	_ = stream.Close()

	// 7. Context cancellation during Next
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream = &sseStream{ctx: ctx}
	_, err = stream.Next()
	if err != context.Canceled {
		t.Errorf("expected context cancelled, got %v", err)
	}
}

func TestSSEStream_Close(t *testing.T) {
	// Test Close when resp is nil
	stream := &sseStream{cancel: func() {}}
	if err := stream.Close(); err != nil {
		t.Errorf("Close with nil resp failed: %v", err)
	}
}