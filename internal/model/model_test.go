package model

import (
	"context"
	"testing"

	"fritz/internal/tool"
)

func TestGenerateFuncImplementsGateway(t *testing.T) {
	gateway := GenerateFunc(func(_ context.Context, req Request) (Response, error) {
		return Response{Text: req.Messages[0].Text()}, nil
	})

	resp, err := gateway.Generate(context.Background(), Request{
		Messages: []Message{TextMessage(UserRole, "hi")},
		ModelID:  "m",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hi" {
		t.Fatalf("Response.Text = %q", resp.Text)
	}
}

func TestStreamGenerateFuncImplementsGateway(t *testing.T) {
	var chunks []string
	var reasoning []string

	gateway := StreamGenerateFunc(func(_ context.Context, req Request, emit func(StreamEvent) error) (Response, error) {
		_ = emit(StreamEvent{ReasoningDelta: "thinking"})
		_ = emit(StreamEvent{TextDelta: "he"})
		_ = emit(StreamEvent{TextDelta: "llo"})
		return Response{Text: req.Messages[0].Text()}, nil
	})

	resp, err := gateway.StreamGenerate(context.Background(), Request{
		Messages: []Message{TextMessage(UserRole, "hello")},
	}, func(event StreamEvent) error {
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
	if len(reasoning) != 1 || reasoning[0] != "thinking" {
		t.Fatalf("reasoning = %#v", reasoning)
	}
}

func TestEstimateRequestTokensIncludesPromptMessagesAndTools(t *testing.T) {
	req := Request{
		SystemPrompt: "system",
		Messages: []Message{
			TextMessage(UserRole, "hello"),
			{
				Role:  ModelRole,
				Parts: []Part{{ToolCall: &tool.Call{ID: "1", Name: "read", Args: map[string]any{"path": "README.md"}}}},
			},
		},
		Tools: []tool.Definition{{Name: "read", Description: "Read file"}},
	}
	if got := EstimateRequestTokens(req); got <= 0 {
		t.Fatalf("EstimateRequestTokens() = %d", got)
	}
}
