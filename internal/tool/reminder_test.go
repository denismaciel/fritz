package tool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReminderToolsUseCurrentSessionPath(t *testing.T) {
	cwd := t.TempDir()
	registry := NewRegistry()
	registry.Register(NewReminderSetToolWithNow(cwd, func() time.Time {
		return time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC)
	}))
	registry.Register(NewReminderListTool(cwd))
	registry.Register(NewReminderDeleteTool(cwd))

	ctx := WithRunContext(context.Background(), RunContext{
		SessionPath: "/tmp/current-session.jsonl",
	})
	setResult, err := registry.Run(ctx, Call{
		ID:   "1",
		Name: "reminder_set",
		Args: map[string]any{
			"when": "2h",
			"text": "send the invoice",
		},
	})
	if err != nil {
		t.Fatalf("reminder_set error = %v", err)
	}
	if !strings.Contains(setResult.Text(), "Reminder") {
		t.Fatalf("setResult = %q", setResult.Text())
	}

	listResult, err := registry.Run(context.Background(), Call{
		ID:   "2",
		Name: "reminder_list",
		Args: map[string]any{},
	})
	if err != nil {
		t.Fatalf("reminder_list error = %v", err)
	}
	if !strings.Contains(listResult.Text(), "send the invoice") || !strings.Contains(listResult.Text(), "/tmp/current-session.jsonl") {
		t.Fatalf("listResult = %q", listResult.Text())
	}
}
