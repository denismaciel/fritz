package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/model"
)

type fakeGateway struct {
	response string
	err      error
}

func (f fakeGateway) Generate(context.Context, model.Request) (model.Response, error) {
	if f.err != nil {
		return model.Response{}, f.err
	}
	return model.Response{Message: model.TextMessage(model.ModelRole, f.response), Text: f.response}, nil
}

func (f fakeGateway) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	resp, err := f.Generate(ctx, req)
	if err != nil {
		return model.Response{}, err
	}
	if resp.Text != "" {
		if err := emit(model.StreamEvent{TextDelta: resp.Text}); err != nil {
			return model.Response{}, err
		}
	}
	return resp, nil
}

func TestManagerCreateAppendOpen(t *testing.T) {
	cwd := t.TempDir()
	manager, err := Create(cwd, ".fritz/sessions")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if manager.SessionFile() == "" {
		t.Fatal("expected session file")
	}
	if _, err := manager.AppendPrompt("hi"); err != nil {
		t.Fatalf("AppendPrompt() error = %v", err)
	}
	if _, err := manager.AppendModelResponse(model.Response{
		Message: model.TextMessage(model.ModelRole, "hello"),
		Text:    "hello",
	}); err != nil {
		t.Fatalf("AppendModelResponse() error = %v", err)
	}
	opened, err := Open(manager.SessionFile(), ".fritz/sessions")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	context := opened.BuildContext()
	if len(context.Transcript) != 1 || context.Transcript[0].Assistant != "hello" {
		t.Fatalf("Transcript = %#v", context.Transcript)
	}
}

func TestManagerBranchTreeAndBranchedSession(t *testing.T) {
	cwd := t.TempDir()
	manager, err := Create(cwd, ".fritz/sessions")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	first, _ := manager.AppendPrompt("one")
	_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "A"), Text: "A"})
	second, _ := manager.AppendPrompt("two")
	_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "B"), Text: "B"})

	if err := manager.MoveLeaf(first.ID); err != nil {
		t.Fatalf("MoveLeaf() error = %v", err)
	}
	_, _ = manager.AppendPrompt("forked")
	_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "C"), Text: "C"})
	if len(manager.Tree()) == 0 {
		t.Fatal("expected tree")
	}
	path, err := manager.CreateBranchedSession(second.ID)
	if err != nil {
		t.Fatalf("CreateBranchedSession() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
}

func TestContinueListAndForkFrom(t *testing.T) {
	cwd := t.TempDir()
	manager, _ := Create(cwd, ".fritz/sessions")
	_, _ = manager.AppendPrompt("hi")
	_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "hello"), Text: "hello"})

	continued, err := ContinueRecent(cwd, ".fritz/sessions")
	if err != nil {
		t.Fatalf("ContinueRecent() error = %v", err)
	}
	if continued.SessionFile() != manager.SessionFile() {
		t.Fatalf("SessionFile = %q", continued.SessionFile())
	}
	list, err := List(cwd, ".fritz/sessions")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() = %#v", list)
	}

	other := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	forked, err := ForkFrom(manager.SessionFile(), other, ".fritz/sessions")
	if err != nil {
		t.Fatalf("ForkFrom() error = %v", err)
	}
	if forked.Header().ParentSession != manager.SessionFile() {
		t.Fatalf("ParentSession = %q", forked.Header().ParentSession)
	}
}

func TestCompactionAndBranchSummary(t *testing.T) {
	t.Run("compaction", func(t *testing.T) {
		cwd := t.TempDir()
		manager, _ := Create(cwd, ".fritz/sessions")
		for _, turn := range []chat.Turn{
			{User: "u1", Assistant: "a1"},
			{User: "u2", Assistant: "a2"},
			{User: "u3", Assistant: "a3"},
		} {
			_, _ = manager.AppendPrompt(turn.User)
			_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, turn.Assistant), Text: turn.Assistant})
		}
		prep, ok := PrepareCompaction(manager, 1)
		if !ok || prep.FirstKeptEntryID == "" {
			t.Fatalf("PrepareCompaction() = %#v %v", prep, ok)
		}
		_, summary, err := Compact(context.Background(), manager, fakeGateway{response: "summary"}, "m", 1, "")
		if err != nil {
			t.Fatalf("Compact() error = %v", err)
		}
		if summary != "summary" {
			t.Fatalf("summary = %q", summary)
		}
		sessionContext := manager.BuildContext()
		if len(sessionContext.Messages) == 0 || !strings.Contains(sessionContext.Messages[0].Text(), "summary") {
			t.Fatalf("Messages = %#v", sessionContext.Messages)
		}
		entries := manager.Entries()
		last := entries[len(entries)-1]
		if last.Type != CompactionEntryType || len(last.ReplacementMessages) == 0 {
			t.Fatalf("compaction entry = %#v", last)
		}
	})

	t.Run("branch summary", func(t *testing.T) {
		cwd := t.TempDir()
		manager, _ := Create(cwd, ".fritz/sessions")
		firstPrompt, _ := manager.AppendPrompt("u1")
		_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "a1"), Text: "a1"})
		_, _ = manager.AppendPrompt("u2")
		_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "a2"), Text: "a2"})
		mainLeaf := manager.LeafID()
		_ = manager.MoveLeaf(firstPrompt.ID)
		_, _ = manager.AppendPrompt("branch user")
		_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "branch answer"), Text: "branch answer"})
		oldLeaf := manager.LeafID()
		branchSummary, err := GenerateBranchSummary(context.Background(), manager, oldLeaf, mainLeaf, fakeGateway{response: "branch summary"}, "m")
		if err != nil {
			t.Fatalf("GenerateBranchSummary() error = %v", err)
		}
		if branchSummary != "branch summary" {
			t.Fatalf("branchSummary = %q", branchSummary)
		}
	})
}

func TestMaybeAutoCompactAndOverflowRetry(t *testing.T) {
	cwd := t.TempDir()
	manager, _ := Create(cwd, ".fritz/sessions")
	for _, turn := range []chat.Turn{
		{User: "u1", Assistant: "a1"},
		{User: "u2", Assistant: "a2"},
		{User: "u3", Assistant: "a3"},
	} {
		_, _ = manager.AppendPrompt(turn.User)
		_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, turn.Assistant), Text: turn.Assistant})
	}
	cfg := config.SessionConfig{Enabled: true, AutoCompact: true, CompactThresholdTurns: 2, CompactKeepTurns: 1, CompactThresholdTokens: 1000, CompactTargetTokens: 100}
	compacted, err := MaybeAutoCompact(context.Background(), manager, cfg, fakeGateway{response: "auto summary"}, "m")
	if err != nil {
		t.Fatalf("MaybeAutoCompact() error = %v", err)
	}
	if !compacted {
		t.Fatal("expected compaction")
	}
	if !ShouldRetryAfterContextOverflow(errors.New("context overflow")) {
		t.Fatal("expected overflow detection")
	}
	retryManager, _ := Create(t.TempDir(), ".fritz/sessions")
	for _, turn := range []chat.Turn{
		{User: "u1", Assistant: "a1"},
		{User: "u2", Assistant: "a2"},
		{User: "u3", Assistant: "a3"},
	} {
		_, _ = retryManager.AppendPrompt(turn.User)
		_, _ = retryManager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, turn.Assistant), Text: turn.Assistant})
	}
	resp, retried, err := RetryAfterOverflow(context.Background(), retryManager, cfg, fakeGateway{response: "retry summary"}, model.Request{ModelID: "m"}, func(req model.Request) (model.Response, error) {
		return model.Response{Message: model.TextMessage(model.ModelRole, "retry ok"), Text: "retry ok"}, nil
	})
	if err != nil {
		t.Fatalf("RetryAfterOverflow() error = %v", err)
	}
	if !retried || resp.Text != "retry ok" {
		t.Fatalf("resp = %#v retried=%v", resp, retried)
	}
}

func TestBuildContextUsesLatestCompactionReplacementMessages(t *testing.T) {
	cwd := t.TempDir()
	manager, _ := Create(cwd, ".fritz/sessions")
	_, _ = manager.AppendPrompt("u1")
	_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "a1"), Text: "a1"})
	_, _ = manager.AppendPrompt("u2")
	_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "a2"), Text: "a2"})
	_, _ = manager.AppendCompaction(
		"summary",
		"",
		0,
		[]model.Message{
			model.TextMessage(model.UserRole, "summary handoff"),
			model.TextMessage(model.UserRole, "u2"),
			model.TextMessage(model.ModelRole, "a2"),
		},
	)
	_, _ = manager.AppendPrompt("u3")
	_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "a3"), Text: "a3"})

	context := manager.BuildContext()
	if len(context.Messages) < 4 {
		t.Fatalf("Messages = %#v", context.Messages)
	}
	if context.Messages[0].Text() != "summary handoff" {
		t.Fatalf("Messages = %#v", context.Messages)
	}
	if got := context.Transcript[len(context.Transcript)-1]; got.User != "u3" || got.Assistant != "a3" {
		t.Fatalf("Transcript = %#v", context.Transcript)
	}
}

func TestRuntimeHost(t *testing.T) {
	cwd := t.TempDir()
	cfg := config.Resolve(config.Sources{Defaults: config.DefaultSource()})
	runtime, err := Start(context.Background(), cwd, cfg, StartOptions{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_, _ = runtime.Manager.AppendPrompt("hello")
	_, _ = runtime.Manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "world"), Text: "world"})
	host := NewHost(cwd, cfg, runtime)
	first := host.Runtime().Manager.SessionFile()
	if err := host.NewSession(context.Background(), first); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if host.Runtime().Manager.Header().ParentSession != first {
		t.Fatalf("ParentSession = %q", host.Runtime().Manager.Header().ParentSession)
	}
	if err := host.SwitchSession(context.Background(), first); err != nil {
		t.Fatalf("SwitchSession() error = %v", err)
	}
	if host.Runtime().Manager.SessionFile() != first {
		t.Fatalf("SessionFile = %q", host.Runtime().Manager.SessionFile())
	}
}
