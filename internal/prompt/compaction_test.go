package prompt

import (
	"strings"
	"testing"

	"fritz/internal/chat"
)

func TestBuildCompactionPromptIncludesTemplateAndTranscript(t *testing.T) {
	text := BuildCompactionPrompt(
		chat.Transcript{{User: "u1", Assistant: "a1"}},
		"Preserve exact next step.",
	)
	if !strings.Contains(text, "CONTEXT CHECKPOINT COMPACTION") {
		t.Fatalf("prompt = %q", text)
	}
	if !strings.Contains(text, "User: u1") || !strings.Contains(text, "Assistant: a1") {
		t.Fatalf("prompt = %q", text)
	}
	if !strings.Contains(text, "Preserve exact next step.") {
		t.Fatalf("prompt = %q", text)
	}
}

func TestBuildCompactionSummaryMessageUsesPrefix(t *testing.T) {
	text := BuildCompactionSummaryMessage("done")
	if !strings.Contains(text, "Another language model compacted the earlier session context.") {
		t.Fatalf("summary = %q", text)
	}
	if !strings.Contains(text, "done") {
		t.Fatalf("summary = %q", text)
	}
	if !IsCompactionSummaryMessage(text) {
		t.Fatalf("expected compaction summary message")
	}
}
