package session

import (
	"encoding/json"
	"testing"

	"fritz/internal/chat"
	"fritz/internal/config"
)

func TestSessionIsSerializable(t *testing.T) {
	session := Session{
		Transcript: chat.Transcript{
			{User: "hi", Assistant: "hello"},
		},
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(data) == "" {
		t.Fatal("expected json")
	}
}

func TestDecideStartMode(t *testing.T) {
	if DecideStartMode(true, true) != ResumeExisting {
		t.Fatal("expected resume")
	}
	if DecideStartMode(false, true) != StartNew {
		t.Fatal("expected new when nothing to resume")
	}
}

func TestPlanPersistenceAndCompaction(t *testing.T) {
	cfg := config.SessionConfig{
		Enabled:                true,
		AutoCompact:            true,
		Dir:                    ".fritz/sessions",
		CompactThresholdTurns:  3,
		CompactKeepTurns:       2,
		CompactThresholdTokens: 1000,
		CompactTargetTokens:    500,
	}
	transcript := chat.Transcript{
		{User: "1", Assistant: "a"},
		{User: "2", Assistant: "b"},
		{User: "3", Assistant: "c"},
		{User: "4", Assistant: "d"},
	}

	persistence := PlanPersistence(cfg, transcript)
	if !persistence.ShouldPersist {
		t.Fatal("expected persistence")
	}

	compaction := PlanCompaction(cfg, transcript, 100)
	if !compaction.ShouldCompact {
		t.Fatal("expected compaction")
	}
	if compaction.DropTurns != 2 {
		t.Fatalf("DropTurns = %d", compaction.DropTurns)
	}

	tokenCompaction := PlanCompaction(cfg, transcript[:2], 1000)
	if tokenCompaction.ShouldCompact || tokenCompaction.Reason != "token_threshold" {
		t.Fatalf("token compaction = %#v", tokenCompaction)
	}

	tokenCompaction = PlanCompaction(cfg, transcript, 1000)
	if !tokenCompaction.ShouldCompact || tokenCompaction.Reason != "token_threshold" {
		t.Fatalf("token compaction = %#v", tokenCompaction)
	}
}
