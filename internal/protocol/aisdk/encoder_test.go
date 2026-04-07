package aisdk

import (
	"testing"

	"fritz/internal/agent"
	"fritz/internal/model"
)

func TestEncoderTextSequence(t *testing.T) {
	encoder := NewEncoder()
	events := []agent.Event{
		{Kind: agent.EventRunStarted, MessageID: "msg-1"},
		{Kind: agent.EventStepStarted, MessageID: "msg-1"},
		{Kind: agent.EventTextDelta, MessageID: "msg-1", TextDelta: "hi"},
		{Kind: agent.EventMessageCompleted, MessageID: "msg-1", Message: messagePtr(model.TextMessage(model.ModelRole, "hi"))},
		{Kind: agent.EventStepFinished, MessageID: "msg-1"},
		{Kind: agent.EventRunFinished, MessageID: "msg-1"},
	}
	var got []string
	for _, event := range events {
		for _, payload := range encoder.Encode(event) {
			got = append(got, payload["type"].(string))
		}
	}
	want := []string{"start", "start-step", "text-start", "text-delta", "text-end", "finish-step", "finish"}
	if len(got) != len(want) {
		t.Fatalf("got = %#v want = %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

func TestEncoderReasoningSequence(t *testing.T) {
	encoder := NewEncoder()
	events := []agent.Event{
		{Kind: agent.EventReasoningStarted, MessageID: "reason-1"},
		{Kind: agent.EventReasoningDelta, MessageID: "reason-1", TextDelta: "think"},
		{Kind: agent.EventReasoningCompleted, MessageID: "reason-1"},
	}
	var got []string
	for _, event := range events {
		for _, payload := range encoder.Encode(event) {
			got = append(got, payload["type"].(string))
		}
	}
	want := []string{"reasoning-start", "reasoning-delta", "reasoning-end"}
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
