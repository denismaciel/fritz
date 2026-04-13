package openaicodex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fritz/internal/model"
	"fritz/internal/provider"
	"fritz/internal/tool"
)

func TestClientSendsCodexHeadersAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "acct_123" {
			t.Fatalf("chatgpt-account-id = %q", got)
		}
		if got := r.Header.Get("OpenAI-Beta"); got != "responses=experimental" {
			t.Fatalf("OpenAI-Beta = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["model"] != "gpt-5.4" {
			t.Fatalf("model = %#v", body["model"])
		}
		if body["instructions"] != "sys" {
			t.Fatalf("instructions = %#v", body["instructions"])
		}
		if body["service_tier"] != "priority" {
			t.Fatalf("service_tier = %#v", body["service_tier"])
		}
		input, _ := body["input"].([]any)
		if len(input) != 1 {
			t.Fatalf("input = %#v", body["input"])
		}
		firstInput, _ := input[0].(map[string]any)
		content, _ := firstInput["content"].([]any)
		if len(content) != 2 {
			t.Fatalf("content = %#v", firstInput["content"])
		}
		secondContent, _ := content[1].(map[string]any)
		if secondContent["type"] != "input_image" {
			t.Fatalf("content[1] = %#v", secondContent)
		}
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("tools = %#v", body["tools"])
		}
		firstTool, _ := tools[0].(map[string]any)
		params, _ := firstTool["parameters"].(map[string]any)
		if _, ok := params["properties"]; !ok {
			t.Fatalf("parameters missing properties: %#v", params)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer server.Close()

	client := NewClient(func(context.Context) (provider.RequestAuth, error) {
		return provider.RequestAuth{
			BearerToken: "token-123",
			AccountID:   "acct_123",
			Headers: map[string]string{
				"originator": "fritz",
			},
		}, nil
	}, WithEndpoint(server.URL+"/backend-api"), WithModel("gpt-5.4"), WithHTTPClient(server.Client()))

	resp, err := client.Generate(context.Background(), model.Request{
		SystemPrompt: "sys",
		Messages: []model.Message{model.MessageWithImages(model.UserRole, "hi", []tool.ContentPart{
			tool.ImagePart("image/png", "Zm9v"),
		})},
		Tools: []tool.Definition{{
			Name:        "read",
			Description: "Read file",
			Parameters: tool.Parameters{
				Type:       "object",
				Properties: map[string]tool.Property{},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "done" {
		t.Fatalf("Text = %q", resp.Text)
	}
}

func TestDecodeSSEBuildsResponse(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"reasoning"}}`,
		"",
		`data: {"type":"response.reasoning_summary_text.delta","delta":"think"}`,
		"",
		`data: {"type":"response.reasoning_summary_part.done"}`,
		"",
		`data: {"type":"response.output_item.added","item":{"type":"message"}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"hel"}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"lo"}`,
		"",
		`data: {"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"hello"}]}}`,
		"",
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_call_1","call_id":"call-1","name":"read","arguments":"{\"path\":\"REA"}}`,
		"",
		`data: {"type":"response.function_call_arguments.delta","delta":"DME.md\"}"}`,
		"",
		`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"fc_call_1","call_id":"call-1","name":"read"}}`,
		"",
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		"",
	}, "\n")

	var gotText strings.Builder
	var gotReasoning strings.Builder
	resp, err := decodeSSE(strings.NewReader(body), func(event model.StreamEvent) error {
		gotText.WriteString(event.TextDelta)
		gotReasoning.WriteString(event.ReasoningDelta)
		return nil
	})
	if err != nil {
		t.Fatalf("decodeSSE() error = %v", err)
	}
	if gotText.String() != "hello" {
		t.Fatalf("TextDelta = %q", gotText.String())
	}
	if !strings.Contains(gotReasoning.String(), "think") {
		t.Fatalf("ReasoningDelta = %q", gotReasoning.String())
	}
	if resp.Text != "hello" {
		t.Fatalf("resp.Text = %q", resp.Text)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read" || resp.ToolCalls[0].Args["path"] != "README.md" {
		t.Fatalf("ToolCalls = %#v", resp.ToolCalls)
	}
	if resp.Message.ReasoningText() == "" {
		t.Fatalf("ReasoningText() = %q", resp.Message.ReasoningText())
	}
}
