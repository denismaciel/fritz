package httpapi

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"fritz/internal/agent"
	"fritz/internal/config"
	"fritz/internal/engine"
	"fritz/internal/model"
	"fritz/internal/tool"
	"net/http/httptest"
)

func TestRenderAGUIRun(t *testing.T) {
	dir := t.TempDir()
	service := agent.NewService(dir, testConfig(), func(_ config.Runtime) model.Client {
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

	var out bytes.Buffer
	summary, err := RenderAGUIRun(context.Background(), server.Client(), server.URL, RunRequest{Prompt: "hi"}, &out)
	if err != nil {
		t.Fatalf("RenderAGUIRun() error = %v", err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("out = %q", out.String())
	}
	if summary.RunID == "" {
		t.Fatalf("summary = %#v", summary)
	}
}
