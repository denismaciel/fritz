package agui

import (
	"bytes"
	"encoding/json"
	"testing"

	"fritz/internal/agent"
	"fritz/internal/model"
	"fritz/internal/protocol/sse"
	"fritz/internal/tool"
)

func TestEncoderTextSequence(t *testing.T) {
	encoder := NewEncoder()
	events := []agent.Event{
		{Kind: agent.EventRunStarted, RunID: "run-1", MessageID: "msg-1", Session: agent.SessionRef{ID: "s1", Path: "/tmp/s.jsonl"}},
		{Kind: agent.EventStepStarted, RunID: "run-1", Step: 1},
		{Kind: agent.EventTextDelta, RunID: "run-1", MessageID: "msg-1", TextDelta: "he"},
		{Kind: agent.EventTextDelta, RunID: "run-1", MessageID: "msg-1", TextDelta: "llo"},
		{Kind: agent.EventMessageCompleted, RunID: "run-1", MessageID: "msg-1", Message: messagePtr(model.TextMessage(model.ModelRole, "hello"))},
		{Kind: agent.EventStepFinished, RunID: "run-1", Step: 1},
		{Kind: agent.EventRunFinished, RunID: "run-1"},
	}

	var got []string
	for _, event := range events {
		for _, payload := range encoder.Encode(event) {
			got = append(got, payload["type"].(string))
		}
	}

	want := []string{
		"RUN_STARTED",
		"STEP_STARTED",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"STEP_FINISHED",
		"RUN_FINISHED",
	}
	if len(got) != len(want) {
		t.Fatalf("got = %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

func TestEncoderWritesSSEJSON(t *testing.T) {
	encoder := NewEncoder()
	var buf bytes.Buffer
	err := encoder.WriteEvent(&buf, agent.Event{
		Kind:      agent.EventToolCallCompleted,
		RunID:     "run-1",
		MessageID: "msg-1",
		ToolCall:  &tool.Call{ID: "call-1", Name: "read"},
		ToolResult: &tool.Result{
			CallID: "call-1",
			Name:   "read",
			Parts:  []tool.ContentPart{tool.TextPart("ok")},
		},
	})
	if err != nil {
		t.Fatalf("WriteEvent() error = %v", err)
	}

	var payloads []map[string]any
	if err := sse.Read(&buf, func(event sse.Event) error {
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			return err
		}
		payloads = append(payloads, payload)
		return nil
	}); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(payloads) != 1 || payloads[0]["type"] != "TOOL_CALL_RESULT" {
		t.Fatalf("payloads = %#v", payloads)
	}
}

func TestEncoderReasoningSequence(t *testing.T) {
	encoder := NewEncoder()
	events := []agent.Event{
		{Kind: agent.EventReasoningStarted, RunID: "run-1", MessageID: "reason-1"},
		{Kind: agent.EventReasoningDelta, RunID: "run-1", MessageID: "reason-1", TextDelta: "think"},
		{Kind: agent.EventReasoningCompleted, RunID: "run-1", MessageID: "reason-1"},
	}
	var got []string
	for _, event := range events {
		for _, payload := range encoder.Encode(event) {
			got = append(got, payload["type"].(string))
		}
	}
	want := []string{"REASONING_START", "REASONING_MESSAGE_START", "REASONING_MESSAGE_CONTENT", "REASONING_MESSAGE_END", "REASONING_END"}
	if len(got) != len(want) {
		t.Fatalf("got = %#v want = %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

func messagePtr(message model.Message) *model.Message {
	return &message
}
