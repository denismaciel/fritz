package model

import (
	"context"
	"encoding/json"
	"strings"

	"fritz/internal/tool"
)

type Role string

const (
	UserRole  Role = "user"
	ModelRole Role = "model"
)

type Part struct {
	Text             string       `json:"text,omitempty"`
	Thought          bool         `json:"thought,omitempty"`
	ToolCall         *tool.Call   `json:"toolCall,omitempty"`
	ToolResult       *tool.Result `json:"toolResult,omitempty"`
	ThoughtSignature string       `json:"thoughtSignature,omitempty"`
}

type Message struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts"`
}

func TextMessage(role Role, text string) Message {
	return Message{
		Role: role,
		Parts: []Part{
			{Text: text},
		},
	}
}

func (m Message) Text() string {
	var out strings.Builder
	for _, part := range m.Parts {
		if part.Text != "" && !part.Thought {
			out.WriteString(part.Text)
		}
	}
	return out.String()
}

func (m Message) ReasoningText() string {
	var out strings.Builder
	for _, part := range m.Parts {
		if part.Text != "" && part.Thought {
			out.WriteString(part.Text)
		}
	}
	return out.String()
}

type Request struct {
	SystemPrompt string
	Messages     []Message
	Tools        []tool.Definition
	ModelID      string
}

const ApproxBytesPerToken = 4

type Response struct {
	Message   Message
	Text      string
	ToolCalls []tool.Call
}

type StreamEvent struct {
	TextDelta      string
	ReasoningDelta string
}

type Gateway interface {
	Generate(ctx context.Context, req Request) (Response, error)
	StreamGenerate(ctx context.Context, req Request, emit func(StreamEvent) error) (Response, error)
}

type GenerateFunc func(ctx context.Context, req Request) (Response, error)

func (f GenerateFunc) Generate(ctx context.Context, req Request) (Response, error) {
	return f(ctx, req)
}

func (f GenerateFunc) StreamGenerate(ctx context.Context, req Request, emit func(StreamEvent) error) (Response, error) {
	resp, err := f(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if resp.Text != "" {
		if err := emit(StreamEvent{TextDelta: resp.Text}); err != nil {
			return Response{}, err
		}
	}
	return resp, nil
}

type StreamGenerateFunc func(ctx context.Context, req Request, emit func(StreamEvent) error) (Response, error)

func (f StreamGenerateFunc) Generate(ctx context.Context, req Request) (Response, error) {
	var text string
	resp, err := f(ctx, req, func(event StreamEvent) error {
		text += event.TextDelta
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	if resp.Text == "" {
		resp.Text = text
	}
	return resp, nil
}

func (f StreamGenerateFunc) StreamGenerate(ctx context.Context, req Request, emit func(StreamEvent) error) (Response, error) {
	return f(ctx, req, emit)
}

func ApproxTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + ApproxBytesPerToken - 1) / ApproxBytesPerToken
}

func EstimateMessageTokens(msg Message) int {
	total := 0
	for _, part := range msg.Parts {
		if part.Text != "" {
			total += ApproxTokenCount(part.Text)
		}
		if part.ToolCall != nil {
			if data, err := json.Marshal(part.ToolCall); err == nil {
				total += ApproxTokenCount(string(data))
			}
		}
		if part.ToolResult != nil {
			if data, err := json.Marshal(part.ToolResult); err == nil {
				total += ApproxTokenCount(string(data))
			}
		}
	}
	return total
}

func EstimateMessagesTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateMessageTokens(msg)
	}
	return total
}

func EstimateToolsTokens(defs []tool.Definition) int {
	if len(defs) == 0 {
		return 0
	}
	data, err := json.Marshal(defs)
	if err != nil {
		return 0
	}
	return ApproxTokenCount(string(data))
}

func EstimateRequestTokens(req Request) int {
	total := ApproxTokenCount(req.SystemPrompt)
	total += EstimateMessagesTokens(req.Messages)
	total += EstimateToolsTokens(req.Tools)
	return total
}
