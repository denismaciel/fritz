package chat

import (
	"testing"

	"fritz/internal/model"
	"fritz/internal/tool"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		line string
		kind InputKind
		text string
	}{
		{line: "", kind: InputEmpty},
		{line: "   ", kind: InputEmpty},
		{line: ":help", kind: InputHelp},
		{line: ":reset", kind: InputReset},
		{line: ":quit", kind: InputQuit},
		{line: "hi", kind: InputPrompt, text: "hi"},
	}

	for _, tt := range tests {
		input := ParseInput(tt.line)
		if input.Kind != tt.kind || input.Text != tt.text {
			t.Fatalf("ParseInput(%q) = %#v", tt.line, input)
		}
	}
}

func TestTranscriptBuildPrompt(t *testing.T) {
	transcript := Transcript{
		{User: "hi", Assistant: "hello"},
	}

	got := transcript.BuildPrompt("what now")
	if got != "Conversation so far:\nUser: hi\nAssistant: hello\nUser: what now" {
		t.Fatalf("BuildPrompt() = %q", got)
	}
}

func TestStartChatEmitsHelp(t *testing.T) {
	result := StartChat(NewState(), true)
	if len(result.Effects) != 4 {
		t.Fatalf("effects = %d", len(result.Effects))
	}
}

func TestHandleInputResetAndQuit(t *testing.T) {
	state := State{
		Transcript: Transcript{{User: "hi", Assistant: "hello"}},
	}

	reset := HandleInput(state, ":reset")
	if len(reset.State.Transcript) != 0 {
		t.Fatalf("reset transcript = %#v", reset.State.Transcript)
	}
	if len(reset.Effects) != 1 {
		t.Fatalf("reset effects = %d", len(reset.Effects))
	}

	quit := HandleInput(state, ":quit")
	if !quit.Exit {
		t.Fatal("expected quit")
	}
}

func TestSubmitPromptAndHandleModelResponse(t *testing.T) {
	state := NewState()

	result := SubmitPrompt(state, "hi")
	if result.State.PendingPrompt != "hi" {
		t.Fatalf("pending = %q", result.State.PendingPrompt)
	}

	call, ok := result.Effects[0].(CallModel)
	if !ok || len(call.Messages) != 1 || call.Messages[0].Text() != "hi" {
		t.Fatalf("effect = %#v", result.Effects[0])
	}

	next := HandleModelResponse(result.State, model.Response{
		Message: model.TextMessage(model.ModelRole, "hello"),
		Text:    "hello",
	})
	if next.State.PendingPrompt != "" {
		t.Fatalf("pending after response = %q", next.State.PendingPrompt)
	}
	if len(next.State.Transcript) != 1 {
		t.Fatalf("transcript len = %d", len(next.State.Transcript))
	}
	printEffect, ok := next.Effects[0].(Print)
	if !ok || printEffect.Line != "hello" {
		t.Fatalf("effect = %#v", next.Effects[0])
	}
}

func TestHandleModelToolCallAndToolResult(t *testing.T) {
	state := SubmitPrompt(NewState(), "read file").State

	response := model.Response{
		Message: model.Message{
			Role: model.ModelRole,
			Parts: []model.Part{
				{
					ToolCall: &tool.Call{
						ID:   "call-1",
						Name: "read",
						Args: map[string]any{"path": "README.md"},
					},
				},
			},
		},
		ToolCalls: []tool.Call{
			{ID: "call-1", Name: "read", Args: map[string]any{"path": "README.md"}},
		},
	}

	next := HandleModelResponse(state, response)
	if len(next.Effects) != 1 {
		t.Fatalf("effects = %d", len(next.Effects))
	}
	runTool, ok := next.Effects[0].(RunTool)
	if !ok || runTool.Call.Name != "read" {
		t.Fatalf("effect = %#v", next.Effects[0])
	}

	afterTool := HandleToolResult(next.State, tool.Result{
		CallID: "call-1",
		Name:   "read",
		Parts:  []tool.ContentPart{tool.TextPart("file text")},
	})
	if len(afterTool.State.Messages) != 3 {
		t.Fatalf("messages = %d", len(afterTool.State.Messages))
	}
	callModel, ok := afterTool.Effects[0].(CallModel)
	if !ok || len(callModel.Messages) != 3 {
		t.Fatalf("effect = %#v", afterTool.Effects[0])
	}
}
