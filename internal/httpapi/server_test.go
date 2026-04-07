package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fritz/internal/agent"
	"fritz/internal/config"
	"fritz/internal/engine"
	"fritz/internal/model"
	"fritz/internal/protocol/sse"
	"fritz/internal/tool"
)

func TestRunsEndpointStreamsAGUI(t *testing.T) {
	dir := t.TempDir()
	service := agent.NewService(dir, testConfig(), func(_ config.Runtime) model.Gateway {
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

	server := httptest.NewServer(NewHandler(engine.WrapService(service)))
	defer server.Close()

	resp, err := http.Post(server.URL+"/runs", "application/json", strings.NewReader(`{"prompt":"hi"}`))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer resp.Body.Close()

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}

	var kinds []string
	if err := sse.Read(resp.Body, func(event sse.Event) error {
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			return err
		}
		kinds = append(kinds, payload["type"].(string))
		return nil
	}); err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(kinds) == 0 || kinds[0] != "RUN_STARTED" {
		t.Fatalf("kinds = %#v", kinds)
	}
	if kinds[len(kinds)-1] != "RUN_FINISHED" {
		t.Fatalf("kinds = %#v", kinds)
	}
}

func TestCancelEndpointStopsRun(t *testing.T) {
	dir := t.TempDir()
	service := agent.NewService(dir, testConfig(), func(_ config.Runtime) model.Gateway {
		return &testGateway{
			streamFunc: func(ctx context.Context, _ model.Request, _ func(model.StreamEvent) error) (model.Response, error) {
				<-ctx.Done()
				return model.Response{}, ctx.Err()
			},
		}
	}, func(config.Runtime) *tool.Registry {
		return tool.NewRegistry()
	})

	runtime, err := service.NewRuntime(context.Background(), agent.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	handle, err := runtime.Submit(context.Background(), agent.RunRequest{Prompt: "wait"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/runs/"+handle.ID+"/cancel", nil)
	rec := httptest.NewRecorder()
	NewHandler(engine.WrapService(service)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Code = %d body=%q", rec.Code, rec.Body.String())
	}
	select {
	case result := <-handle.Done:
		if result.Err == nil {
			t.Fatal("expected cancel error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled run")
	}
}

func TestRunsEndpointStreamsToolFlow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello from file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	service := agent.NewService(dir, testConfig(), func(_ config.Runtime) model.Gateway {
		return &toolGateway{}
	}, func(config.Runtime) *tool.Registry {
		registry := tool.NewRegistry()
		registry.Register(tool.NewReadTool(dir, 1024))
		return registry
	})

	server := httptest.NewServer(NewHandler(engine.WrapService(service)))
	defer server.Close()

	resp, err := http.Post(server.URL+"/runs", "application/json", strings.NewReader(`{"prompt":"read README"}`))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer resp.Body.Close()

	var kinds []string
	if err := sse.Read(resp.Body, func(event sse.Event) error {
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			return err
		}
		kinds = append(kinds, payload["type"].(string))
		return nil
	}); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertHasType(t, kinds, "TOOL_CALL_START")
	assertHasType(t, kinds, "TOOL_CALL_RESULT")
	assertHasType(t, kinds, "RUN_FINISHED")
}

func TestRunsEndpointStreamsError(t *testing.T) {
	dir := t.TempDir()
	service := agent.NewService(dir, testConfig(), func(_ config.Runtime) model.Gateway {
		return &testGateway{
			streamFunc: func(_ context.Context, _ model.Request, _ func(model.StreamEvent) error) (model.Response, error) {
				return model.Response{}, errors.New("boom")
			},
			generateFunc: func(_ context.Context, _ model.Request) (model.Response, error) {
				return model.Response{}, errors.New("boom")
			},
		}
	}, func(config.Runtime) *tool.Registry {
		return tool.NewRegistry()
	})

	server := httptest.NewServer(NewHandler(engine.WrapService(service)))
	defer server.Close()

	resp, err := http.Post(server.URL+"/runs", "application/json", strings.NewReader(`{"prompt":"hi"}`))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer resp.Body.Close()

	var kinds []string
	if err := sse.Read(resp.Body, func(event sse.Event) error {
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			return err
		}
		kinds = append(kinds, payload["type"].(string))
		return nil
	}); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertHasType(t, kinds, "RUN_ERROR")
}

func TestAISDKEndpointStreamsDataProtocol(t *testing.T) {
	dir := t.TempDir()
	service := agent.NewService(dir, testConfig(), func(_ config.Runtime) model.Gateway {
		return &testGateway{
			streamFunc: func(_ context.Context, _ model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
				_ = emit(model.StreamEvent{TextDelta: "ok"})
				return model.Response{
					Message: model.TextMessage(model.ModelRole, "ok"),
					Text:    "ok",
				}, nil
			},
		}
	}, func(config.Runtime) *tool.Registry {
		return tool.NewRegistry()
	})

	server := httptest.NewServer(NewHandler(engine.WrapService(service)))
	defer server.Close()

	resp, err := http.Post(server.URL+"/ai-sdk/chat", "application/json", strings.NewReader(`{"messages":[{"role":"user","parts":[{"type":"text","text":"hi"}]}]}`))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("x-vercel-ai-ui-message-stream"); got != "v1" {
		t.Fatalf("header = %q", got)
	}
	var types []string
	if err := sse.Read(resp.Body, func(event sse.Event) error {
		if event.Data == "[DONE]" {
			types = append(types, "[DONE]")
			return nil
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			return err
		}
		types = append(types, payload["type"].(string))
		return nil
	}); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(types) == 0 || types[0] != "start" || types[len(types)-1] != "[DONE]" {
		t.Fatalf("types = %#v", types)
	}
}

type testGateway struct {
	streamFunc   func(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error)
	generateFunc func(ctx context.Context, req model.Request) (model.Response, error)
}

type toolGateway struct {
	calls int
}

func (g *testGateway) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if g.generateFunc != nil {
		return g.generateFunc(ctx, req)
	}
	return model.Response{}, nil
}

func (g *testGateway) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	return g.streamFunc(ctx, req, emit)
}

func (g *toolGateway) Generate(_ context.Context, _ model.Request) (model.Response, error) {
	g.calls++
	if g.calls == 1 {
		call := tool.Call{ID: "call-1", Name: "read", Args: map[string]any{"path": "README.md"}}
		return model.Response{
			Message: model.Message{
				Role:  model.ModelRole,
				Parts: []model.Part{{ToolCall: &call}},
			},
			ToolCalls: []tool.Call{call},
		}, nil
	}
	return model.Response{
		Message: model.TextMessage(model.ModelRole, "done"),
		Text:    "done",
	}, nil
}

func (g *toolGateway) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	return g.Generate(ctx, req)
}

func testConfig() config.Runtime {
	return config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Env: config.Source{
			GeminiAPIKey: "test-key",
			Session: config.SessionConfigSource{
				Enabled: boolPtr(false),
			},
		},
	})
}

func boolPtr(v bool) *bool { return &v }

func assertHasType(t *testing.T, kinds []string, want string) {
	t.Helper()
	for _, kind := range kinds {
		if kind == want {
			return
		}
	}
	t.Fatalf("types %#v missing %q", kinds, want)
}
