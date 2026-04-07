package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/model"
	"fritz/internal/session"
	"fritz/internal/tool"
)

func TestCompactionParity(t *testing.T) {
	t.Run("snapshot_request_shape_pre_turn_compaction_context_window_exceeded", func(t *testing.T) {
		dir := t.TempDir()
		cfg := compactingTestConfig(t, dir)
		var requests []model.Request
		service := NewService(dir, cfg, func(_ config.Runtime) model.Gateway {
			return &testGateway{
				generateFunc: func(req model.Request, call int) (model.Response, error) {
					requests = append(requests, req)
					if call == 1 {
						return model.Response{Message: model.TextMessage(model.ModelRole, "summary"), Text: "summary"}, nil
					}
					return model.Response{Message: model.TextMessage(model.ModelRole, "done"), Text: "done"}, nil
				},
			}
		}, func(config.Runtime) *tool.Registry {
			return tool.NewRegistry()
		})
		runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
		if err != nil {
			t.Fatalf("NewRuntime() error = %v", err)
		}
		seedRuntimeTurns(t, runtime, chat.Turn{User: "older", Assistant: strings.Repeat("a", 256)}, chat.Turn{User: "older-2", Assistant: strings.Repeat("b", 256)})

		handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "next"})
		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		_, result := collectRun(t, handle)
		if result.Err != nil {
			t.Fatalf("result.Err = %v", result.Err)
		}
		if len(requests) < 2 || !strings.Contains(requests[0].Messages[0].Text(), "CONTEXT CHECKPOINT COMPACTION") {
			t.Fatalf("requests = %#v", requests)
		}
		if !requestHasSummaryPrefix(requests[1]) {
			t.Fatalf("follow-up request = %#v", requests[1].Messages)
		}
	})

	t.Run("snapshot_request_shape_mid_turn_continuation_compaction", func(t *testing.T) {
		dir := t.TempDir()
		cfg := testConfig(t, dir)
		cfg.Session.Dir = ".fritz/sessions"
		cfg.Session.CompactThresholdTokens = 3000
		cfg.Session.CompactTargetTokens = 256
		cfg.Session.CompactKeepTurns = 1
		cfg.Session.CompactThresholdTurns = 100
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(strings.Repeat("z", 20000)), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		var requests []model.Request
		service := NewService(dir, cfg, func(_ config.Runtime) model.Gateway {
			return &testGateway{
				generateFunc: func(req model.Request, call int) (model.Response, error) {
					requests = append(requests, req)
					switch call {
					case 1:
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
					case 2:
						return model.Response{Message: model.TextMessage(model.ModelRole, "summary"), Text: "summary"}, nil
					default:
						return model.Response{Message: model.TextMessage(model.ModelRole, "done"), Text: "done"}, nil
					}
				},
			}
		}, func(config.Runtime) *tool.Registry {
			registry := tool.NewRegistry()
			registry.Register(tool.NewReadTool(dir, 30000))
			return registry
		})
		runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
		if err != nil {
			t.Fatalf("NewRuntime() error = %v", err)
		}
		seedRuntimeTurns(t, runtime, chat.Turn{User: "older", Assistant: "a1"}, chat.Turn{User: "older-2", Assistant: "a2"})

		handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "read it"})
		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		_, result := collectRun(t, handle)
		if result.Err != nil {
			t.Fatalf("result.Err = %v", result.Err)
		}
		if len(requests) < 3 {
			t.Fatalf("requests = %#v", requests)
		}
		if !strings.Contains(requests[1].Messages[0].Text(), "CONTEXT CHECKPOINT COMPACTION") {
			t.Fatalf("compaction request = %#v", requests[1])
		}
		if !requestHasSummaryPrefix(requests[2]) || !requestHasToolResultOrOmission(requests[2]) {
			t.Fatalf("continuation request = %#v", requests[2].Messages)
		}
	})

	t.Run("multiple_auto_compact_per_task_runs_after_token_limit_hit", func(t *testing.T) {
		dir := t.TempDir()
		cfg := compactingTestConfig(t, dir)
		var requests []model.Request
		service := NewService(dir, cfg, func(_ config.Runtime) model.Gateway {
			return &testGateway{
				generateFunc: func(req model.Request, call int) (model.Response, error) {
					requests = append(requests, req)
					if call%2 == 1 {
						return model.Response{Message: model.TextMessage(model.ModelRole, "summary"), Text: "summary"}, nil
					}
					return model.Response{Message: model.TextMessage(model.ModelRole, "done"), Text: "done"}, nil
				},
			}
		}, func(config.Runtime) *tool.Registry {
			return tool.NewRegistry()
		})
		runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{})
		if err != nil {
			t.Fatalf("NewRuntime() error = %v", err)
		}
		seedRuntimeTurns(t, runtime, chat.Turn{User: "older", Assistant: strings.Repeat("a", 256)}, chat.Turn{User: "older-2", Assistant: strings.Repeat("b", 256)})

		for _, prompt := range []string{"next", "next-again"} {
			handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: prompt})
			if err != nil {
				t.Fatalf("Submit() error = %v", err)
			}
			_, result := collectRun(t, handle)
			if result.Err != nil {
				t.Fatalf("result.Err = %v", result.Err)
			}
		}
		compactionCalls := 0
		for _, req := range requests {
			if len(req.Messages) > 0 && strings.Contains(req.Messages[0].Text(), "CONTEXT CHECKPOINT COMPACTION") {
				compactionCalls++
			}
		}
		if compactionCalls < 2 {
			t.Fatalf("requests = %#v", requests)
		}
	})

	t.Run("auto_compact_runs_after_resume_when_token_usage_is_over_limit", func(t *testing.T) {
		dir := t.TempDir()
		cfg := compactingTestConfig(t, dir)
		seedPersistedRuntimeManager(t, dir)

		var requests []model.Request
		service := NewService(dir, cfg, func(_ config.Runtime) model.Gateway {
			return &testGateway{
				generateFunc: func(req model.Request, call int) (model.Response, error) {
					requests = append(requests, req)
					if call == 1 {
						return model.Response{Message: model.TextMessage(model.ModelRole, "summary"), Text: "summary"}, nil
					}
					return model.Response{Message: model.TextMessage(model.ModelRole, "done"), Text: "done"}, nil
				},
			}
		}, func(config.Runtime) *tool.Registry {
			return tool.NewRegistry()
		})
		runtime, err := service.NewRuntime(context.Background(), RuntimeOptions{
			Session: sessionOptionsContinue(),
		})
		if err != nil {
			t.Fatalf("NewRuntime() error = %v", err)
		}
		handle, err := runtime.Submit(context.Background(), RunRequest{Prompt: "resume-next"})
		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		_, result := collectRun(t, handle)
		if result.Err != nil {
			t.Fatalf("result.Err = %v", result.Err)
		}
		if len(requests) < 2 || !strings.Contains(requests[0].Messages[0].Text(), "CONTEXT CHECKPOINT COMPACTION") {
			t.Fatalf("requests = %#v", requests)
		}
	})
}

func compactingTestConfig(t *testing.T, dir string) config.Runtime {
	t.Helper()
	cfg := testConfig(t, dir)
	cfg.Session.Dir = ".fritz/sessions"
	cfg.Session.CompactThresholdTokens = 1
	cfg.Session.CompactTargetTokens = 128
	cfg.Session.CompactKeepTurns = 1
	cfg.Session.CompactThresholdTurns = 100
	return cfg
}

func seedRuntimeTurns(t *testing.T, runtime *Runtime, turns ...chat.Turn) {
	t.Helper()
	for _, turn := range turns {
		if _, err := runtime.manager.AppendPrompt(turn.User); err != nil {
			t.Fatalf("AppendPrompt() error = %v", err)
		}
		if _, err := runtime.manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, turn.Assistant), Text: turn.Assistant}); err != nil {
			t.Fatalf("AppendModelResponse() error = %v", err)
		}
		runtime.state.Transcript = append(runtime.state.Transcript, turn)
		runtime.state.Messages = append(runtime.state.Messages,
			model.TextMessage(model.UserRole, turn.User),
			model.TextMessage(model.ModelRole, turn.Assistant),
		)
	}
}

func seedPersistedRuntimeManager(t *testing.T, dir string) {
	t.Helper()
	cfg := compactingTestConfig(t, dir)
	runtime, err := NewService(dir, cfg, nil, nil).NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	seedRuntimeTurns(t, runtime, chat.Turn{User: "older", Assistant: strings.Repeat("a", 256)}, chat.Turn{User: "older-2", Assistant: strings.Repeat("b", 256)})
}

func sessionOptionsContinue() session.StartOptions {
	return session.StartOptions{Continue: true}
}

func requestHasSummaryPrefix(req model.Request) bool {
	for _, msg := range req.Messages {
		if strings.Contains(msg.Text(), "Another language model compacted the earlier session context.") {
			return true
		}
	}
	return false
}

func requestHasToolResultOrOmission(req model.Request) bool {
	for _, msg := range req.Messages {
		if strings.Contains(msg.Text(), "tool result omitted after compaction") {
			return true
		}
		for _, part := range msg.Parts {
			if part.ToolResult != nil {
				return true
			}
		}
	}
	return false
}
