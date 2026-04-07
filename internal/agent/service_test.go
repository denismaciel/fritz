package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fritz/internal/config"
	"fritz/internal/model"
	"fritz/internal/prompt"
	"fritz/internal/tool"
)

func TestRuntimeSubmitStreamsTextEvents(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, dir)
	service := NewService(dir, cfg, func(_ config.Runtime) model.Client {
		return &testGateway{
			streamFunc: func(_ context.Context, _ model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
				_ = emit(model.StreamEvent{TextDelta: "he"})
				_ = emit(model.StreamEvent{TextDelta: "llo"})
				return model.Response{
					Message: model.TextMessage(model.ModelRole, "hello"),
					Text:    "hello",
				}, nil
			},
		}
	}, func(config.Runtime) *tool.Registry {
		return tool.NewRegistry()
	})

	runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	events, result := collectRun(t, handle)
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	assertKinds(t, events,
		EventRunStarted,
		EventStepStarted,
		EventTextDelta,
		EventTextDelta,
		EventMessageCompleted,
		EventStepFinished,
		EventRunFinished,
	)
	if got := collectTextDeltas(events); got != "hello" {
		t.Fatalf("text deltas = %q", got)
	}
	if result.State.Transcript[len(result.State.Transcript)-1].Assistant != "hello" {
		t.Fatalf("assistant = %#v", result.State.Transcript)
	}
}

func TestRuntimeSubmitStreamsReasoningEvents(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, dir)
	service := NewService(dir, cfg, func(_ config.Runtime) model.Client {
		return &testGateway{
			streamFunc: func(_ context.Context, _ model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
				_ = emit(model.StreamEvent{ReasoningDelta: "reason "})
				_ = emit(model.StreamEvent{ReasoningDelta: "done"})
				_ = emit(model.StreamEvent{TextDelta: "answer"})
				return model.Response{
					Message: model.Message{
						Role: model.ModelRole,
						Parts: []model.Part{
							{Text: "reason done", Thought: true},
							{Text: "answer"},
						},
					},
					Text: "answer",
				}, nil
			},
		}
	}, func(config.Runtime) *tool.Registry {
		return tool.NewRegistry()
	})

	runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	events, result := collectRun(t, handle)
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	assertKinds(t, events,
		EventRunStarted,
		EventStepStarted,
		EventReasoningStarted,
		EventReasoningDelta,
		EventReasoningDelta,
		EventTextDelta,
		EventReasoningCompleted,
		EventMessageCompleted,
		EventStepFinished,
		EventRunFinished,
	)
}

func TestRuntimeSubmitEmitsToolEvents(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello from file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	service := NewService(dir, cfg, func(_ config.Runtime) model.Client {
		return &testGateway{
			generateFunc: func(req model.Request, call int) (model.Response, error) {
				if call == 1 {
					return model.Response{
						Message: model.Message{
							Role: model.ModelRole,
							Parts: []model.Part{{ToolCall: &tool.Call{
								ID:   "call-1",
								Name: "read",
								Args: map[string]any{"path": "README.md"},
							}}},
						},
						ToolCalls: []tool.Call{{ID: "call-1", Name: "read", Args: map[string]any{"path": "README.md"}}},
					}, nil
				}
				return model.Response{
					Message: model.TextMessage(model.ModelRole, "done"),
					Text:    "done",
				}, nil
			},
		}
	}, func(config.Runtime) *tool.Registry {
		registry := tool.NewRegistry()
		registry.Register(tool.NewReadTool(dir, 1024))
		return registry
	})

	runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "read it"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	events, result := collectRun(t, handle)
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}

	assertKinds(t, events,
		EventRunStarted,
		EventStepStarted,
		EventToolCallStarted,
		EventToolCallCompleted,
		EventStepFinished,
		EventStepStarted,
		EventMessageCompleted,
		EventStepFinished,
		EventRunFinished,
	)

	var sawRead bool
	for _, event := range events {
		if event.Kind == EventToolCallCompleted && event.ToolResult != nil && strings.Contains(event.ToolResult.Text(), "hello from file") {
			sawRead = true
		}
	}
	if !sawRead {
		t.Fatalf("events = %#v", events)
	}
}

func TestServiceCancelStopsActiveRun(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, dir)
	service := NewService(dir, cfg, func(_ config.Runtime) model.Client {
		return &testGateway{
			streamFunc: func(ctx context.Context, _ model.Request, _ func(model.StreamEvent) error) (model.Response, error) {
				<-ctx.Done()
				return model.Response{}, ctx.Err()
			},
		}
	}, func(config.Runtime) *tool.Registry {
		return tool.NewRegistry()
	})

	runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "wait"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if !service.Cancel(handle.ID) {
		t.Fatal("Cancel() = false")
	}

	events, result := collectRun(t, handle)
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("result.Err = %v", result.Err)
	}
	if kinds := eventKinds(events); kinds[len(kinds)-1] != EventRunCanceled {
		t.Fatalf("kinds = %#v", kinds)
	}
}

func TestRuntimeSubmitCompactsBeforeModelCallWhenTokenThresholdHit(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, dir)
	cfg.Session.CompactThresholdTokens = 1
	cfg.Session.CompactTargetTokens = 64
	cfg.Session.CompactKeepTurns = 1
	cfg.Session.CompactThresholdTurns = 100

	var requests []model.Request
	service := NewService(dir, cfg, func(_ config.Runtime) model.Client {
		return &testGateway{
			generateFunc: func(req model.Request, call int) (model.Response, error) {
				requests = append(requests, req)
				if call == 1 {
					return model.Response{
						Message: model.TextMessage(model.ModelRole, "summary"),
						Text:    "summary",
					}, nil
				}
				return model.Response{
					Message: model.TextMessage(model.ModelRole, "done"),
					Text:    "done",
				}, nil
			},
		}
	}, func(config.Runtime) *tool.Registry {
		return tool.NewRegistry()
	})

	runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	_, _ = runtime.manager.AppendPrompt("older")
	_, _ = runtime.manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, strings.Repeat("a", 512)), Text: strings.Repeat("a", 512)})
	_, _ = runtime.manager.AppendPrompt("older-2")
	_, _ = runtime.manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, strings.Repeat("b", 512)), Text: strings.Repeat("b", 512)})

	handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "next"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	_, result := collectRun(t, handle)
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}
	if len(requests) < 2 {
		t.Fatalf("requests = %#v", requests)
	}
	if !strings.Contains(requests[0].Messages[0].Text(), "CONTEXT CHECKPOINT COMPACTION") {
		t.Fatalf("first request = %#v", requests[0])
	}
	foundSummary := false
	for _, msg := range requests[1].Messages {
		if strings.Contains(msg.Text(), "Another language model compacted the earlier session context.") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("follow-up request = %#v", requests[1].Messages)
	}
}

func TestRuntimeSubmitUsesCodingPromptProfileByDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("remember"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("check"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var request model.Request
	service := NewService(dir, cfg, func(_ config.Runtime) model.Client {
		return &testGateway{
			generateFunc: func(req model.Request, _ int) (model.Response, error) {
				request = req
				return model.Response{
					Message: model.TextMessage(model.ModelRole, "done"),
					Text:    "done",
				}, nil
			},
		}
	}, func(config.Runtime) *tool.Registry {
		registry := tool.NewRegistry()
		registry.Register(tool.NewReadTool(dir, 1024))
		return registry
	})

	runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	_, result := collectRun(t, handle)
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}
	if strings.Contains(request.SystemPrompt, "# Durable Memory") || strings.Contains(request.SystemPrompt, "# Heartbeat Context") {
		t.Fatalf("coding prompt leaked gateway sections: %q", request.SystemPrompt)
	}
}

func TestRuntimeSubmitUsesGatewayPromptProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("remember"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("check"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var request model.Request
	service := NewServiceWithPromptProfile(dir, cfg, func(_ config.Runtime) model.Client {
		return &testGateway{
			generateFunc: func(req model.Request, _ int) (model.Response, error) {
				request = req
				return model.Response{
					Message: model.TextMessage(model.ModelRole, "done"),
					Text:    "done",
				}, nil
			},
		}
	}, func(config.Runtime) *tool.Registry {
		registry := tool.NewRegistry()
		registry.Register(tool.NewReadTool(dir, 1024))
		return registry
	}, prompt.ProfileGateway)

	runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	_, result := collectRun(t, handle)
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}
	if !strings.Contains(request.SystemPrompt, "# Durable Memory") || !strings.Contains(request.SystemPrompt, "# Heartbeat Context") {
		t.Fatalf("gateway prompt missing memory/heartbeat sections: %q", request.SystemPrompt)
	}
}

type testGateway struct {
	generateFunc func(req model.Request, call int) (model.Response, error)
	streamFunc   func(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error)
	calls        int
}

func (g *testGateway) Generate(_ context.Context, req model.Request) (model.Response, error) {
	g.calls++
	if g.generateFunc != nil {
		return g.generateFunc(req, g.calls)
	}
	return model.Response{}, nil
}

func (g *testGateway) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	if g.streamFunc != nil {
		return g.streamFunc(ctx, req, emit)
	}
	return g.Generate(ctx, req)
}

func testConfig(t *testing.T, dir string) config.Runtime {
	t.Helper()
	return config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Env: config.Source{
			GeminiAPIKey: "test-key",
			Session: config.SessionConfigSource{
				Dir: dir,
			},
		},
	})
}

func collectRun(t *testing.T, handle RunHandle) ([]Event, RunResult) {
	t.Helper()
	var events []Event
	for event := range handle.Events {
		events = append(events, event)
	}
	select {
	case result := <-handle.Done:
		return events, result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run result")
	}
	return nil, RunResult{}
}

func eventKinds(events []Event) []EventKind {
	out := make([]EventKind, 0, len(events))
	for _, event := range events {
		out = append(out, event.Kind)
	}
	return out
}

func assertKinds(t *testing.T, events []Event, want ...EventKind) {
	t.Helper()
	got := eventKinds(events)
	if len(got) != len(want) {
		t.Fatalf("kinds len = %d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds[%d] = %q want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func collectTextDeltas(events []Event) string {
	var out strings.Builder
	for _, event := range events {
		if event.Kind == EventTextDelta {
			out.WriteString(event.TextDelta)
		}
	}
	return out.String()
}
