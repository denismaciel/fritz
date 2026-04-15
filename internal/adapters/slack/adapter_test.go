package slack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fritz/internal/config"
	"fritz/internal/engine"
	"fritz/internal/ingress"
)

type fakeClient struct {
	posts          []PostMessageRequest
	prompts        [][]SuggestedPrompt
	uploads        []UploadFileRequest
	startStreamErr error
	streamStarts   int
	streamAppends  []string
	streamStops    int
	statuses       []string
	setTitles      []string
}

func (f *fakeClient) OpenSocketConnection(context.Context) (*SocketConn, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeClient) PostMessage(_ context.Context, req PostMessageRequest) error {
	f.posts = append(f.posts, req)
	return nil
}

func (f *fakeClient) StartStream(context.Context, string, string) (StreamHandle, error) {
	f.streamStarts++
	if f.startStreamErr != nil {
		return StreamHandle{}, f.startStreamErr
	}
	return StreamHandle{Channel: "C1", TS: "1.1"}, nil
}

func (f *fakeClient) AppendStream(_ context.Context, _ StreamHandle, text string) error {
	f.streamAppends = append(f.streamAppends, text)
	return nil
}

func (f *fakeClient) StopStream(context.Context, StreamHandle) error {
	f.streamStops++
	return nil
}
func (f *fakeClient) SetStatus(_ context.Context, _ AssistantThreadRef, status string) error {
	f.statuses = append(f.statuses, status)
	return nil
}
func (f *fakeClient) SetTitle(_ context.Context, _ AssistantThreadRef, title string) error {
	f.setTitles = append(f.setTitles, title)
	return nil
}
func (f *fakeClient) SetSuggestedPrompts(_ context.Context, _ AssistantThreadRef, prompts []SuggestedPrompt) error {
	f.prompts = append(f.prompts, prompts)
	return nil
}
func (f *fakeClient) ConversationsReplies(context.Context, string, string) ([]HistoryMessage, error) {
	return nil, nil
}
func (f *fakeClient) UploadFile(_ context.Context, req UploadFileRequest) error {
	f.uploads = append(f.uploads, req)
	return nil
}

type fakeHandler struct {
	cleared []string
	result  ingress.HandleResult
}

func (f *fakeHandler) HandleInbound(context.Context, ingress.InboundMessage) (ingress.HandleResult, error) {
	return f.result, nil
}

func (f *fakeHandler) HandleInboundStream(_ context.Context, _ ingress.InboundMessage, emit func(engine.Event) error) (ingress.HandleResult, error) {
	if emit != nil {
		_ = emit(engine.Event{Kind: engine.EventTextDelta, TextDelta: "hello"})
	}
	return f.result, nil
}

func (f *fakeHandler) ClearSessionKey(_ context.Context, sessionKey string) error {
	f.cleared = append(f.cleared, sessionKey)
	return nil
}

func TestHandleMessageEventClearSession(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAdapterWithPaths(ingress.ResolveStatePaths(dir, testRuntimeConfig()), &fakeClient{}, &fakeHandler{}, Config{})
	client := adapter.client.(*fakeClient)
	handler := adapter.handler.(*fakeHandler)

	err := adapter.handleMessageEvent(context.Background(), "T1", "E1", Event{
		Type:        "message",
		User:        "U1",
		Text:        "/clear",
		Channel:     "D1",
		ChannelType: "im",
		TS:          "10.1",
	}, map[string]any{})
	if err != nil {
		t.Fatalf("handleMessageEvent() error = %v", err)
	}
	if len(handler.cleared) != 1 || handler.cleared[0] != "slack:im:T1:D1:10.1" {
		t.Fatalf("cleared = %#v", handler.cleared)
	}
	if len(client.posts) != 1 || client.posts[0].Text != "history cleared" {
		t.Fatalf("posts = %#v", client.posts)
	}
}

func TestHandleAssistantThreadStartedStoresContextAndPrompts(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAdapterWithPaths(ingress.ResolveStatePaths(dir, testRuntimeConfig()), &fakeClient{}, &fakeHandler{}, Config{Assistant: true})
	client := adapter.client.(*fakeClient)

	err := adapter.handleAssistantThreadStarted(context.Background(), "T1", map[string]any{
		"assistant_thread": map[string]any{
			"channel_id": "D1",
			"thread_ts":  "20.1",
			"context": map[string]any{
				"channel_name": "general",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleAssistantThreadStarted() error = %v", err)
	}
	state, err := adapter.loadContexts()
	if err != nil {
		t.Fatalf("loadContexts() error = %v", err)
	}
	if len(state.Bindings) != 1 || state.Bindings[0].RouteKey != "slack:assistant:T1:D1:20.1" {
		t.Fatalf("bindings = %#v", state.Bindings)
	}
	if len(client.prompts) != 1 || len(client.prompts[0]) == 0 {
		t.Fatalf("prompts = %#v", client.prompts)
	}
}

func TestUploadArtifactsUploadsFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memo.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	client := &fakeClient{}
	adapter := NewAdapterWithPaths(ingress.ResolveStatePaths(dir, testRuntimeConfig()), client, &fakeHandler{}, Config{})
	if err := adapter.uploadArtifacts(context.Background(), root, "C1", "30.1"); err != nil {
		t.Fatalf("uploadArtifacts() error = %v", err)
	}
	if len(client.uploads) != 1 || client.uploads[0].Filename != "memo.pdf" {
		t.Fatalf("uploads = %#v", client.uploads)
	}
}

func TestStreamRendererFallsBackToPostMessage(t *testing.T) {
	client := &fakeClient{}
	renderer := newFinalRenderer(client, "C1", "40.1")
	if err := renderer.Emit(engine.Event{Kind: engine.EventTextDelta, TextDelta: "hello"}); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	if err := renderer.Finish(context.Background(), ingress.HandleResult{}); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if len(client.posts) != 1 || client.posts[0].Text != "hello" {
		t.Fatalf("posts = %#v", client.posts)
	}
	if client.streamStarts != 0 || client.streamStops != 0 || len(client.streamAppends) != 0 {
		t.Fatalf("stream calls = starts:%d stops:%d appends:%#v", client.streamStarts, client.streamStops, client.streamAppends)
	}
	if len(client.posts[0].Blocks) == 0 {
		t.Fatalf("blocks = %#v", client.posts[0].Blocks)
	}
}

func TestMarkdownToSlackMessagesRendersBlocks(t *testing.T) {
	rendered := markdownToSlackMessages("# Title\n\n- one\n- two\n\n```go\nfmt.Println(\"hi\")\n```")
	if len(rendered) != 1 {
		t.Fatalf("rendered len = %d", len(rendered))
	}
	if rendered[0].Text == "" || len(rendered[0].Blocks) == 0 {
		t.Fatalf("rendered = %#v", rendered)
	}
	if got := flattenRenderedText(rendered); got == "" || !strings.Contains(got, "*Title*") || !strings.Contains(got, "• one") || !strings.Contains(got, "```") {
		t.Fatalf("flattened = %q", got)
	}
}

func TestHandleMessageEventAssistantClearsStatus(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{}
	handler := &fakeHandler{result: ingress.HandleResult{Messages: []ingress.OutboundMessage{{Text: "**done**"}}}}
	adapter := NewAdapterWithPaths(ingress.ResolveStatePaths(dir, testRuntimeConfig()), client, handler, Config{Assistant: true})
	if err := adapter.upsertContext(ingress.SlackContextBinding{
		RouteKey:  "slack:assistant:T1:C1:20.1",
		TeamID:    "T1",
		ChannelID: "C1",
		ThreadTS:  "20.1",
	}); err != nil {
		t.Fatalf("upsertContext() error = %v", err)
	}
	err := adapter.handleMessageEvent(context.Background(), "T1", "E1", Event{
		Type:        "message",
		User:        "U1",
		Text:        "hi",
		Channel:     "C1",
		ChannelType: "im",
		ThreadTS:    "20.1",
		TS:          "20.2",
	}, map[string]any{})
	if err != nil {
		t.Fatalf("handleMessageEvent() error = %v", err)
	}
	if len(client.statuses) < 2 || client.statuses[0] != "Thinking..." || client.statuses[len(client.statuses)-1] != "" {
		t.Fatalf("statuses = %#v", client.statuses)
	}
	if len(client.posts) == 0 || !strings.Contains(client.posts[0].Text, "*done*") {
		t.Fatalf("posts = %#v", client.posts)
	}
}

func testRuntimeConfig() config.Runtime {
	return config.Resolve(config.Sources{Defaults: config.DefaultSource()})
}
