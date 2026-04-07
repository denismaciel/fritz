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
