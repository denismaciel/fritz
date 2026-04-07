package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"fritz/internal/config"
	"fritz/internal/model"
	"fritz/internal/tool"
)

func TestBashContextParity(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	t.Run("should execute bash command", func(t *testing.T) {
		registry := tool.NewRegistry()
		registry.Register(tool.NewBashTool(t.TempDir()))
		var output bytes.Buffer
		var generateCalls int
		err := run(context.Background(), []string{"run", "echo hi"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Gateway {
			return fakeGateway{
				streamDisabled: true,
				generateCalls:  &generateCalls,
				generateFunc: func(req model.Request, call int) model.Response {
					if call == 1 {
						return model.Response{
							Message:   model.Message{Role: model.ModelRole, Parts: []model.Part{{ToolCall: &tool.Call{ID: "call-1", Name: "bash", Args: map[string]any{"command": "printf hi"}}}}},
							ToolCalls: []tool.Call{{ID: "call-1", Name: "bash", Args: map[string]any{"command": "printf hi"}}},
						}
					}
					return model.Response{Message: model.TextMessage(model.ModelRole, "done"), Text: "done"}
				},
			}
		}, registry)
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if !strings.Contains(output.String(), "done") {
			t.Fatalf("output = %q", output.String())
		}
	})

	t.Run("should add bash output to context", func(t *testing.T) {
		registry := tool.NewRegistry()
		registry.Register(tool.NewBashTool(t.TempDir()))
		var sawToolResult bool
		var generateCalls int
		err := run(context.Background(), []string{"run", "echo hi"}, strings.NewReader(""), io.Discard, func(_ config.Runtime) model.Gateway {
			return fakeGateway{
				streamDisabled: true,
				generateCalls:  &generateCalls,
				generateFunc: func(req model.Request, call int) model.Response {
					if call == 1 {
						return model.Response{
							Message:   model.Message{Role: model.ModelRole, Parts: []model.Part{{ToolCall: &tool.Call{ID: "call-1", Name: "bash", Args: map[string]any{"command": "printf hi"}}}}},
							ToolCalls: []tool.Call{{ID: "call-1", Name: "bash", Args: map[string]any{"command": "printf hi"}}},
						}
					}
					for _, message := range req.Messages {
						if strings.Contains(message.Text(), "hi") {
							sawToolResult = true
						}
						for _, part := range message.Parts {
							if part.ToolResult != nil && strings.Contains(part.ToolResult.Text(), "hi") {
								sawToolResult = true
							}
						}
					}
					return model.Response{Message: model.TextMessage(model.ModelRole, "done"), Text: "done"}
				},
			}
		}, registry)
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if !sawToolResult {
			t.Fatal("expected bash output in context")
		}
	})

	t.Run("should include bash output in LLM context", func(t *testing.T) {
		registry := tool.NewRegistry()
		registry.Register(tool.NewBashTool(t.TempDir()))
		var sawOutput bool
		var generateCalls int
		err := run(context.Background(), []string{"run", "echo hi"}, strings.NewReader(""), io.Discard, func(_ config.Runtime) model.Gateway {
			return fakeGateway{
				streamDisabled: true,
				generateCalls:  &generateCalls,
				generateFunc: func(req model.Request, call int) model.Response {
					if call == 1 {
						return model.Response{
							Message:   model.Message{Role: model.ModelRole, Parts: []model.Part{{ToolCall: &tool.Call{ID: "call-1", Name: "bash", Args: map[string]any{"command": "printf 123"}}}}},
							ToolCalls: []tool.Call{{ID: "call-1", Name: "bash", Args: map[string]any{"command": "printf 123"}}},
						}
					}
					for _, message := range req.Messages {
						if strings.Contains(message.Text(), "123") {
							sawOutput = true
						}
						for _, part := range message.Parts {
							if part.ToolResult != nil && strings.Contains(part.ToolResult.Text(), "123") {
								sawOutput = true
							}
						}
					}
					return model.Response{Message: model.TextMessage(model.ModelRole, "123"), Text: "123"}
				},
			}
		}, registry)
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if !sawOutput {
			t.Fatal("expected model context to include bash output")
		}
	})
}
