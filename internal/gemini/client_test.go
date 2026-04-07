package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fritz/internal/model"
	"fritz/internal/tool"
)

func TestClientGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1beta/models/gemini-3-flash-preview:generateContent" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Fatalf("x-goog-api-key = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if _, ok := body["system_instruction"].(map[string]any); !ok {
			t.Fatalf("system_instruction = %#v", body["system_instruction"])
		}

		contents, ok := body["contents"].([]any)
		if !ok || len(contents) != 1 {
			t.Fatalf("contents = %#v", body["contents"])
		}
		tools, ok := body["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v", body["tools"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates": [
				{
					"content": {
						"parts": [
							{"text": "hello back"}
						]
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(
		"test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
	)

	resp, err := client.Generate(context.Background(), model.Request{
		SystemPrompt: "be terse",
		Messages:     []model.Message{model.TextMessage(model.UserRole, "hi there")},
		Tools: []tool.Definition{
			{
				Name:        "read",
				Description: "Read a file",
				Parameters: tool.Parameters{
					Type: "object",
					Properties: map[string]tool.Property{
						"path": {Type: "string"},
					},
					Required: []string{"path"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hello back" {
		t.Fatalf("Generate() = %q", resp.Text)
	}
}

func TestClientGenerateNoText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[]}}]}`))
	}))
	defer server.Close()

	client := NewClient(
		"test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.Generate(context.Background(), model.Request{
		Messages: []model.Message{model.TextMessage(model.UserRole, "hi there")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientStreamGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-3-flash-preview:streamGenerateContent" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "sse" {
			t.Fatalf("alt = %q", r.URL.Query().Get("alt"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"thinking \",\"thought\":true}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"he\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"llo\"}]}}]}\n\n"))
	}))
	defer server.Close()

	client := NewClient(
		"test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
	)

	var chunks []string
	var reasoning []string
	resp, err := client.StreamGenerate(context.Background(), model.Request{
		Messages: []model.Message{model.TextMessage(model.UserRole, "hello")},
	}, func(event model.StreamEvent) error {
		if event.TextDelta != "" {
			chunks = append(chunks, event.TextDelta)
		}
		if event.ReasoningDelta != "" {
			reasoning = append(reasoning, event.ReasoningDelta)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamGenerate() error = %v", err)
	}
	if resp.Text != "hello" {
		t.Fatalf("Response.Text = %q", resp.Text)
	}
	if len(chunks) != 2 || chunks[0] != "he" || chunks[1] != "llo" {
		t.Fatalf("chunks = %#v", chunks)
	}
	if len(reasoning) != 1 || reasoning[0] != "thinking " {
		t.Fatalf("reasoning = %#v", reasoning)
	}
}

func TestClientGenerateFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"functionCall": {
									"id": "call-1",
									"name": "read",
									"args": {"path": "README.md"}
								},
								"thoughtSignature": "sig-1"
							}
						]
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithEndpoint(server.URL), WithHTTPClient(server.Client()))
	resp, err := client.Generate(context.Background(), model.Request{
		Messages: []model.Message{model.TextMessage(model.UserRole, "read it")},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read" {
		t.Fatalf("ToolCalls = %#v", resp.ToolCalls)
	}
	if resp.Message.Parts[0].ThoughtSignature != "sig-1" {
		t.Fatalf("ThoughtSignature = %q", resp.Message.Parts[0].ThoughtSignature)
	}
}

func TestClientGenerateRetries429(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := attempts.Add(1)
		if current == 1 {
			http.Error(w, "rate limit", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
	}))
	defer server.Close()

	client := NewClient(
		"test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithRetryPolicy(2, time.Millisecond),
	)

	resp, err := client.Generate(context.Background(), model.Request{
		Messages: []model.Message{model.TextMessage(model.UserRole, "hi")},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("Response.Text = %q", resp.Text)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
}

func TestClientGenerateDoesNotRetry400(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(
		"test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithRetryPolicy(3, time.Millisecond),
	)

	_, err := client.Generate(context.Background(), model.Request{
		Messages: []model.Message{model.TextMessage(model.UserRole, "hi")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
	if !strings.Contains(err.Error(), "400 Bad Request") {
		t.Fatalf("error = %v", err)
	}
}

func TestShouldRetry(t *testing.T) {
	if !shouldRetry(&RequestError{StatusCode: http.StatusTooManyRequests}) {
		t.Fatal("expected retry for 429")
	}
	if shouldRetry(&RequestError{StatusCode: http.StatusBadRequest}) {
		t.Fatal("unexpected retry for 400")
	}
	if shouldRetry(context.Canceled) {
		t.Fatal("unexpected retry for canceled")
	}
	if !shouldRetry(timeoutNetError{}) {
		t.Fatal("expected retry for net error")
	}
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestRequestErrorIncludesBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Body:       io.NopCloser(strings.NewReader("rate limited")),
	}
	err := newRequestError("generate", resp)
	if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v", err)
	}
}
