package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"fritz/internal/config"
	"fritz/internal/model"
)

func TestLocalServiceStartAndSubmit(t *testing.T) {
	dir := t.TempDir()
	service := NewLocalService(dir, testConfig(dir), func(_ config.Runtime) model.Gateway {
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
	}, NewRegistryFactory(nil))

	session, err := service.Start(context.Background(), SessionOptions{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	run, err := session.Submit(context.Background(), RunRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	events, result := collectRun(t, run)
	if result.Err != nil {
		t.Fatalf("result.Err = %v", result.Err)
	}
	if len(events) == 0 || events[0].Kind != EventRunStarted {
		t.Fatalf("events = %#v", events)
	}
	if got := result.State.Transcript[len(result.State.Transcript)-1].Assistant; got != "hello" {
		t.Fatalf("assistant = %q", got)
	}
}

func TestLocalServiceCancelStopsRun(t *testing.T) {
	dir := t.TempDir()
	service := NewLocalService(dir, testConfig(dir), func(_ config.Runtime) model.Gateway {
		return &testGateway{
			streamFunc: func(ctx context.Context, _ model.Request, _ func(model.StreamEvent) error) (model.Response, error) {
				<-ctx.Done()
				return model.Response{}, ctx.Err()
			},
		}
	}, NewRegistryFactory(nil))

	session, err := service.Start(context.Background(), SessionOptions{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	run, err := session.Submit(context.Background(), RunRequest{Prompt: "wait"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if !service.Cancel(run.ID) {
		t.Fatal("Cancel() = false")
	}

	_, result := collectRun(t, run)
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("result.Err = %v", result.Err)
	}
}

type testGateway struct {
	streamFunc func(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error)
}

func (g *testGateway) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	if g.streamFunc != nil {
		return g.streamFunc(ctx, req, func(model.StreamEvent) error { return nil })
	}
	return model.Response{}, nil
}

func (g *testGateway) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	if g.streamFunc != nil {
		return g.streamFunc(ctx, req, emit)
	}
	return model.Response{}, nil
}

func testConfig(dir string) config.Runtime {
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

func collectRun(t *testing.T, run Run) ([]Event, RunResult) {
	t.Helper()
	events := make([]Event, 0, 8)
	for event := range run.Events {
		events = append(events, event)
	}
	result := <-run.Done
	return events, result
}

var _ Service = (*LocalService)(nil)
var _ Session = localSession{}
