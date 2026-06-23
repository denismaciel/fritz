package session

import (
	"context"
	"strings"
	"testing"

	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/model"
	"fritz/internal/tool"
)

func TestCompactionParity(t *testing.T) {
	t.Run("reconstruct_history_matches_live_compactions", func(t *testing.T) {
		cwd := t.TempDir()
		manager := seededPersistedManager(t, cwd,
			chat.Turn{User: "u1", Assistant: "a1"},
			chat.Turn{User: "u2", Assistant: "a2"},
			chat.Turn{User: "u3", Assistant: "a3"},
		)

		if _, _, err := compactWithOptions(context.Background(), manager, fakeGateway{response: "summary"}, "m", compactOptions{
			keepTurns:    1,
			targetTokens: 256,
		}); err != nil {
			t.Fatalf("compactWithOptions() error = %v", err)
		}

		live := manager.BuildContext()
		reopened, err := Open(manager.SessionFile(), ".fritz/sessions")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		reconstructed := reopened.BuildContext()
		if len(live.Messages) != len(reconstructed.Messages) || len(live.Transcript) != len(reconstructed.Transcript) {
			t.Fatalf("live=%#v reconstructed=%#v", live, reconstructed)
		}
		if live.Messages[0].Text() != reconstructed.Messages[0].Text() {
			t.Fatalf("live=%#v reconstructed=%#v", live.Messages, reconstructed.Messages)
		}
	})

	t.Run("build_token_limited_compacted_history_truncates_overlong_user_messages", func(t *testing.T) {
		manager := seededInMemoryManager(t,
			chat.Turn{User: "older", Assistant: "a1"},
			chat.Turn{User: strings.Repeat("x", 1200), Assistant: "a2"},
		)
		if _, _, err := compactWithOptions(context.Background(), manager, fakeGateway{response: "summary"}, "m", compactOptions{
			keepTurns:    1,
			targetTokens: 120,
		}); err != nil {
			t.Fatalf("compactWithOptions() error = %v", err)
		}
		entry := latestCompactionEntry(t, manager)
		if got := model.EstimateMessagesTokens(entry.ReplacementMessages); got > 120 {
			t.Fatalf("replacement tokens = %d", got)
		}
		found := false
		for _, msg := range entry.ReplacementMessages {
			if strings.Contains(msg.Text(), "[truncated after compaction]") {
				found = true
			}
		}
		if !found {
			t.Fatalf("replacementMessages = %#v", entry.ReplacementMessages)
		}
	})

	t.Run("build_token_limited_compacted_history_appends_summary_message", func(t *testing.T) {
		manager := seededInMemoryManager(t,
			chat.Turn{User: "older", Assistant: "a1"},
			chat.Turn{User: "latest", Assistant: "a2"},
		)
		if _, _, err := compactWithOptions(context.Background(), manager, fakeGateway{response: "summary"}, "m", compactOptions{
			keepTurns:    1,
			targetTokens: 256,
		}); err != nil {
			t.Fatalf("compactWithOptions() error = %v", err)
		}
		entry := latestCompactionEntry(t, manager)
		if len(entry.ReplacementMessages) == 0 || !strings.Contains(entry.ReplacementMessages[0].Text(), "Another language model compacted the earlier session context.") {
			t.Fatalf("replacementMessages = %#v", entry.ReplacementMessages)
		}
	})

	t.Run("manual_compact_uses_custom_prompt", func(t *testing.T) {
		manager := seededInMemoryManager(t,
			chat.Turn{User: "older", Assistant: "a1"},
			chat.Turn{User: "latest", Assistant: "a2"},
		)
		var seen model.Request
		customGateway := fakeGatewayWithCapture{capture: &seen, response: "summary"}
		if _, _, err := Compact(context.Background(), manager, customGateway, "m", 1, "Keep the branch note."); err != nil {
			t.Fatalf("Compact() error = %v", err)
		}
		if strings.TrimSpace(seen.SystemPrompt) == "" {
			t.Fatal("SystemPrompt is empty")
		}
		if len(seen.Messages) == 0 || !strings.Contains(seen.Messages[0].Text(), "Keep the branch note.") {
			t.Fatalf("request = %#v", seen)
		}
	})

	t.Run("auto_compact_persists_rollout_entries", func(t *testing.T) {
		cwd := t.TempDir()
		manager := seededPersistedManager(t, cwd,
			chat.Turn{User: "u1", Assistant: "a1"},
			chat.Turn{User: "u2", Assistant: "a2"},
			chat.Turn{User: "u3", Assistant: "a3"},
		)
		_, _ = manager.AppendToolResult("tool output", model.Message{
			Role: model.UserRole,
			Parts: []model.Part{
				{ToolResult: &tool.Result{CallID: "call-1", Name: "read", Parts: []tool.ContentPart{tool.TextPart("tool output")}}},
			},
		})
		cfg := config.SessionConfig{
			Enabled:                true,
			AutoCompact:            true,
			CompactKeepTurns:       1,
			CompactThresholdTurns:  2,
			CompactThresholdTokens: 256,
			CompactTargetTokens:    128,
		}
		compacted, err := MaybeAutoCompact(context.Background(), manager, cfg, fakeGateway{response: "summary"}, "m")
		if err != nil {
			t.Fatalf("MaybeAutoCompact() error = %v", err)
		}
		if !compacted {
			t.Fatal("expected compaction")
		}
		reopened, err := Open(manager.SessionFile(), ".fritz/sessions")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		entry := latestCompactionEntry(t, reopened)
		if len(entry.ReplacementMessages) == 0 {
			t.Fatalf("entry = %#v", entry)
		}
	})

	t.Run("manual_compact_retries_after_context_window_error", func(t *testing.T) {
		manager := seededInMemoryManager(t,
			chat.Turn{User: "u1", Assistant: "a1"},
			chat.Turn{User: "u2", Assistant: "a2"},
			chat.Turn{User: "u3", Assistant: "a3"},
		)
		cfg := config.SessionConfig{
			Enabled:                true,
			AutoCompact:            true,
			CompactKeepTurns:       1,
			CompactThresholdTurns:  2,
			CompactThresholdTokens: 256,
			CompactTargetTokens:    128,
		}
		called := 0
		resp, retried, err := RetryAfterOverflow(context.Background(), manager, cfg, fakeGateway{response: "retry summary"}, model.Request{ModelID: "m"}, func(req model.Request) (model.Response, error) {
			called++
			return model.Response{Message: model.TextMessage(model.ModelRole, "retry ok"), Text: "retry ok"}, nil
		})
		if err != nil {
			t.Fatalf("RetryAfterOverflow() error = %v", err)
		}
		if !retried || called != 1 || resp.Text != "retry ok" {
			t.Fatalf("resp=%#v retried=%v called=%d", resp, retried, called)
		}
	})

	t.Run("manual_compact_twice_preserves_latest_user_messages", func(t *testing.T) {
		manager := seededInMemoryManager(t,
			chat.Turn{User: "u1", Assistant: "a1"},
			chat.Turn{User: "u2", Assistant: "a2"},
			chat.Turn{User: "u3", Assistant: "a3"},
			chat.Turn{User: "u4", Assistant: "a4"},
		)
		if _, _, err := compactWithOptions(context.Background(), manager, fakeGateway{response: "summary-1"}, "m", compactOptions{
			keepTurns:    2,
			targetTokens: 256,
		}); err != nil {
			t.Fatalf("compactWithOptions() error = %v", err)
		}
		_, _ = manager.AppendPrompt("u5")
		_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "a5"), Text: "a5"})
		if _, _, err := compactWithOptions(context.Background(), manager, fakeGateway{response: "summary-2"}, "m", compactOptions{
			keepTurns:    2,
			targetTokens: 256,
		}); err != nil {
			t.Fatalf("compactWithOptions() error = %v", err)
		}
		context := manager.BuildContext()
		if got := context.Transcript[len(context.Transcript)-2:]; got[0].User != "u4" || got[1].User != "u5" {
			t.Fatalf("transcript = %#v", context.Transcript)
		}
	})

	t.Run("auto_compact_allows_multiple_attempts_when_interleaved_with_other_turn_events", func(t *testing.T) {
		manager := seededInMemoryManager(t,
			chat.Turn{User: "u1", Assistant: "a1"},
			chat.Turn{User: "u2", Assistant: "a2"},
			chat.Turn{User: "u3", Assistant: "a3"},
			chat.Turn{User: "u4", Assistant: "a4"},
		)
		cfg := config.SessionConfig{
			Enabled:                true,
			AutoCompact:            true,
			CompactKeepTurns:       2,
			CompactThresholdTurns:  3,
			CompactThresholdTokens: 256,
			CompactTargetTokens:    128,
		}
		first, err := MaybeAutoCompact(context.Background(), manager, cfg, fakeGateway{response: "summary-1"}, "m")
		if err != nil || !first {
			t.Fatalf("MaybeAutoCompact() = %v %v", first, err)
		}
		_, _ = manager.AppendToolResult("interleaved", model.Message{
			Role:  model.UserRole,
			Parts: []model.Part{{ToolResult: &tool.Result{CallID: "call-2", Name: "read", Parts: []tool.ContentPart{tool.TextPart("interleaved")}}}},
		})
		_, _ = manager.AppendPrompt("u5")
		_, _ = manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, "a5"), Text: "a5"})
		second, err := MaybeAutoCompact(context.Background(), manager, cfg, fakeGateway{response: "summary-2"}, "m")
		if err != nil || !second {
			t.Fatalf("MaybeAutoCompact() = %v %v", second, err)
		}
		if got := manager.BuildContext().Transcript[len(manager.BuildContext().Transcript)-1].User; got != "u5" {
			t.Fatalf("transcript = %#v", manager.BuildContext().Transcript)
		}
	})
}

type fakeGatewayWithCapture struct {
	capture  *model.Request
	response string
}

func (f fakeGatewayWithCapture) Generate(_ context.Context, req model.Request) (model.Response, error) {
	if f.capture != nil {
		*f.capture = req
	}
	return model.Response{Message: model.TextMessage(model.ModelRole, f.response), Text: f.response}, nil
}

func (f fakeGatewayWithCapture) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
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

func seededPersistedManager(t *testing.T, cwd string, turns ...chat.Turn) *Manager {
	t.Helper()
	manager, err := Create(cwd, ".fritz/sessions")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	seedManagerTurns(t, manager, turns...)
	return manager
}

func seededInMemoryManager(t *testing.T, turns ...chat.Turn) *Manager {
	t.Helper()
	manager := InMemory(t.TempDir())
	seedManagerTurns(t, manager, turns...)
	return manager
}

func seedManagerTurns(t *testing.T, manager *Manager, turns ...chat.Turn) {
	t.Helper()
	for _, turn := range turns {
		if _, err := manager.AppendPrompt(turn.User); err != nil {
			t.Fatalf("AppendPrompt() error = %v", err)
		}
		if _, err := manager.AppendModelResponse(model.Response{Message: model.TextMessage(model.ModelRole, turn.Assistant), Text: turn.Assistant}); err != nil {
			t.Fatalf("AppendModelResponse() error = %v", err)
		}
	}
}

func latestCompactionEntry(t *testing.T, manager *Manager) Line {
	t.Helper()
	entries := manager.Entries()
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type == CompactionEntryType {
			return entries[i]
		}
	}
	t.Fatal("missing compaction entry")
	return Line{}
}
