package terminalui

import (
	"strings"
	"testing"

	"fritz/internal/agent"
	"fritz/internal/tool"
)

func TestStateApplyReasoningAndAssistantText(t *testing.T) {
	state := NewState()
	state = state.AddUserPrompt("hi")
	state = state.Apply(agent.Event{Kind: agent.EventReasoningStarted, MessageID: "r1"})
	state = state.Apply(agent.Event{Kind: agent.EventReasoningDelta, MessageID: "r1", TextDelta: "plan "})
	state = state.Apply(agent.Event{Kind: agent.EventReasoningDelta, MessageID: "r1", TextDelta: "more"})
	state = state.Apply(agent.Event{Kind: agent.EventReasoningCompleted, MessageID: "r1"})
	state = state.Apply(agent.Event{Kind: agent.EventTextDelta, MessageID: "a1", TextDelta: "hello"})
	state = state.Apply(agent.Event{Kind: agent.EventMessageCompleted, MessageID: "a1"})

	if len(state.Items) != 3 {
		t.Fatalf("Items = %#v", state.Items)
	}
	if state.Items[1].Kind != ItemReasoning || state.Items[1].Text != "plan more" {
		t.Fatalf("reasoning item = %#v", state.Items[1])
	}
	if state.Items[2].Kind != ItemAssistant || state.Items[2].Text != "hello" {
		t.Fatalf("assistant item = %#v", state.Items[2])
	}
}

func TestAddUserPromptWithImages(t *testing.T) {
	state := NewState()
	state = state.AddUserPromptWithImages("look", []tool.ContentPart{
		tool.ImagePart("image/png", "Zm9v"),
	})
	if len(state.Items) != 1 {
		t.Fatalf("Items = %#v", state.Items)
	}
	if !strings.Contains(state.Items[0].Text, "[Image #1] image/png") {
		t.Fatalf("item = %#v", state.Items[0])
	}
}

func TestStateApplyToolPreview(t *testing.T) {
	state := NewState()
	state = state.Apply(agent.Event{
		Kind:      agent.EventToolCallStarted,
		MessageID: "t1",
		ToolCall:  &tool.Call{Name: "read", Args: map[string]any{"path": "README.md"}},
	})
	state = state.Apply(agent.Event{
		Kind:      agent.EventToolCallCompleted,
		MessageID: "t1",
		ToolCall:  &tool.Call{Name: "read", Args: map[string]any{"path": "README.md"}},
		ToolResult: &tool.Result{
			Name:  "read",
			Parts: []tool.ContentPart{tool.TextPart(strings.Repeat("x", 400))},
		},
	})

	if len(state.Items) != 1 {
		t.Fatalf("Items = %#v", state.Items)
	}
	if state.Items[0].Kind != ItemTool {
		t.Fatalf("tool item = %#v", state.Items[0])
	}
	if len(state.Items[0].Preview) == 0 || len(state.Items[0].Preview) >= 400 {
		t.Fatalf("preview = %q", state.Items[0].Preview)
	}
	if state.Items[0].PreviewIsDiff {
		t.Fatalf("PreviewIsDiff = true")
	}
}

func TestStateApplyEditToolUsesDiffPreview(t *testing.T) {
	state := NewState()
	state = state.Apply(agent.Event{
		Kind:      agent.EventToolCallCompleted,
		MessageID: "t1",
		ToolCall:  &tool.Call{Name: "edit", Args: map[string]any{"path": "README.md"}},
		ToolResult: &tool.Result{
			Name:    "edit",
			Parts:   []tool.ContentPart{tool.TextPart("Successfully replaced 1 block(s) in README.md.")},
			Details: tool.EditResultDetails{Diff: " old\n-before\n+after"},
		},
	})

	if got := state.Items[0].Preview; got != "old\n-before\n+after" {
		t.Fatalf("preview = %q", got)
	}
	if !state.Items[0].PreviewIsDiff {
		t.Fatal("PreviewIsDiff = false")
	}
}

func TestStateApplyWriteToolUsesDiffPreview(t *testing.T) {
	state := NewState()
	state = state.Apply(agent.Event{
		Kind:      agent.EventToolCallCompleted,
		MessageID: "t1",
		ToolCall:  &tool.Call{Name: "write", Args: map[string]any{"path": "README.md"}},
		ToolResult: &tool.Result{
			Name:    "write",
			Parts:   []tool.ContentPart{tool.TextPart("Successfully wrote 5 bytes to README.md.")},
			Details: tool.WriteResultDetails{Diff: " new\n+after"},
		},
	})

	if got := state.Items[0].Preview; got != "new\n+after" {
		t.Fatalf("preview = %q", got)
	}
	if !state.Items[0].PreviewIsDiff {
		t.Fatal("PreviewIsDiff = false")
	}
}

func TestRenderToolTitleHidesEditEdits(t *testing.T) {
	title := renderToolTitle(&tool.Call{
		Name: "edit",
		Args: map[string]any{
			"path":  "README.md",
			"edits": []any{map[string]any{"oldText": "a", "newText": "b"}},
		},
	})
	if strings.Contains(title, "edits=") {
		t.Fatalf("title = %q", title)
	}
	if !strings.Contains(title, "path=") {
		t.Fatalf("title = %q", title)
	}
}

func TestRenderToolTitleHidesWriteContent(t *testing.T) {
	title := renderToolTitle(&tool.Call{
		Name: "write",
		Args: map[string]any{
			"path":    "README.md",
			"content": "hello",
		},
	})
	if strings.Contains(title, "content=") {
		t.Fatalf("title = %q", title)
	}
	if !strings.Contains(title, "path=") {
		t.Fatalf("title = %q", title)
	}
}

func TestRenderItemWrapsLongAssistantText(t *testing.T) {
	rendered := renderItem(Item{
		Kind: ItemAssistant,
		Text: "this is a fairly long line that should wrap in the viewport instead of staying on one endless line",
	}, 24)

	if !strings.Contains(rendered, "\n") {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestRenderItemWrapsToolPreview(t *testing.T) {
	rendered := renderItem(Item{
		Kind:    ItemTool,
		Title:   "read path=\"README.md\"",
		Preview: "this is a very long preview that should wrap to fit the viewport width cleanly",
	}, 28)

	if !strings.Contains(rendered, "\n") {
		t.Fatalf("rendered = %q", rendered)
	}
}
