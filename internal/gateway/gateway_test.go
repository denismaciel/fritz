package gateway

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fritz/internal/config"
	"fritz/internal/engine"
	"fritz/internal/model"
)

func TestGatewayHandleInboundReturnsReply(t *testing.T) {
	dir := t.TempDir()
	gw := New(dir, testConfig(dir), testService(dir))

	result, err := gw.HandleInbound(context.Background(), InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u1",
		Text:     "hi",
	})
	if err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if result.SessionKey != "telegram:dm:u1" {
		t.Fatalf("SessionKey = %q", result.SessionKey)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("Messages len = %d", len(result.Messages))
	}
	if result.Messages[0].Text != "reply: hi" {
		t.Fatalf("Text = %q", result.Messages[0].Text)
	}
	if result.Messages[0].Channel != "telegram" {
		t.Fatalf("Channel = %q", result.Messages[0].Channel)
	}
}

func TestGatewayHandleInboundReusesSessionForSameUser(t *testing.T) {
	dir := t.TempDir()
	gw := New(dir, testConfig(dir), testService(dir))

	first, err := gw.HandleInbound(context.Background(), InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u1",
		Text:     "first",
	})
	if err != nil {
		t.Fatalf("first HandleInbound() error = %v", err)
	}
	second, err := gw.HandleInbound(context.Background(), InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u1",
		Text:     "second",
	})
	if err != nil {
		t.Fatalf("second HandleInbound() error = %v", err)
	}
	if first.Session.Path == "" || second.Session.Path == "" {
		t.Fatalf("missing session path: %+v %+v", first.Session, second.Session)
	}
	if first.Session.Path != second.Session.Path {
		t.Fatalf("session path mismatch: %q != %q", first.Session.Path, second.Session.Path)
	}
	if second.Messages[0].Text != "reply: second | seen: first" {
		t.Fatalf("Text = %q", second.Messages[0].Text)
	}
}

func TestGatewayHandleInboundIsolatesDifferentUsers(t *testing.T) {
	dir := t.TempDir()
	gw := New(dir, testConfig(dir), testService(dir))

	if _, err := gw.HandleInbound(context.Background(), InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u1",
		Text:     "first",
	}); err != nil {
		t.Fatalf("first HandleInbound() error = %v", err)
	}
	second, err := gw.HandleInbound(context.Background(), InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u2",
		Text:     "second",
	})
	if err != nil {
		t.Fatalf("second HandleInbound() error = %v", err)
	}
	if second.Messages[0].Text != "reply: second" {
		t.Fatalf("Text = %q", second.Messages[0].Text)
	}
}

func TestGatewayPersistsSessionMapAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir)

	firstGateway := New(dir, cfg, testService(dir))
	first, err := firstGateway.HandleInbound(context.Background(), InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u1",
		Text:     "first",
	})
	if err != nil {
		t.Fatalf("first HandleInbound() error = %v", err)
	}

	secondGateway := New(dir, cfg, testService(dir))
	second, err := secondGateway.HandleInbound(context.Background(), InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u1",
		Text:     "second",
	})
	if err != nil {
		t.Fatalf("second HandleInbound() error = %v", err)
	}
	if first.Session.Path != second.Session.Path {
		t.Fatalf("session path mismatch: %q != %q", first.Session.Path, second.Session.Path)
	}
}

func TestGatewayStateRootUsesCodingAgentDir(t *testing.T) {
	dir := t.TempDir()
	gw := New(dir, testConfig(dir), testService(dir))

	want := filepath.Join(dir, ".fritz", "gateway")
	if got := gw.StateRoot(); got != want {
		t.Fatalf("StateRoot() = %q, want %q", got, want)
	}
}

func TestGatewayMigratesLegacyStateFile(t *testing.T) {
	dir := t.TempDir()
	gw := New(dir, testConfig(dir), testService(dir))
	if err := os.MkdirAll(gw.StateRoot(), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(gw.StateRoot(), "state.json"), []byte(`{"sessions":{"telegram:dm:u1":"/tmp/legacy.jsonl"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	state, err := gw.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	if state.Sessions["telegram:dm:u1"] != "/tmp/legacy.jsonl" {
		t.Fatalf("state = %#v", state)
	}
	if _, err := os.Stat(gw.Paths().RoutingSessionMapPath); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
}

func testService(dir string) engine.Service {
	return engine.NewLocalService(
		dir,
		testConfig(dir),
		engine.NewGatewayFactory(func(config.Runtime) model.Gateway {
			return testGateway{}
		}),
		nil,
	)
}

func testConfig(dir string) config.Runtime {
	source := config.DefaultSource()
	source.Session.Dir = filepath.Join(dir, ".fritz", "sessions")
	return config.Resolve(config.Sources{Defaults: source})
}

type testGateway struct{}

func (testGateway) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	last := req.Messages[len(req.Messages)-1].Text()
	response := "reply: " + last
	if len(req.Messages) >= 3 {
		response += " | seen: " + req.Messages[len(req.Messages)-3].Text()
	}
	return model.Response{
		Message: model.TextMessage(model.ModelRole, response),
		Text:    response,
	}, nil
}

func (g testGateway) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	resp, err := g.Generate(ctx, req)
	if err != nil {
		return model.Response{}, err
	}
	if err := emit(model.StreamEvent{TextDelta: resp.Text}); err != nil {
		return model.Response{}, err
	}
	return resp, nil
}
