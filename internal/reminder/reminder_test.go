package reminder

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSetListDeleteAndDue(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "reminders.json"))
	now := time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC)

	first, err := store.Set(now, ReminderInput{
		SessionPath: "/tmp/session-a.jsonl",
		When:        now.Add(time.Hour),
		Text:        "pay bill",
	})
	if err != nil {
		t.Fatalf("Set(first) error = %v", err)
	}
	_, err = store.Set(now, ReminderInput{
		SessionPath: "/tmp/session-b.jsonl",
		When:        now.Add(-time.Minute),
		Text:        "call mom",
	})
	if err != nil {
		t.Fatalf("Set(second) error = %v", err)
	}

	items, err := store.List()
	if err != nil || len(items) != 2 {
		t.Fatalf("List() = %#v, %v", items, err)
	}

	due, err := store.ClaimDue(now)
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if len(due) != 1 || due[0].Text != "call mom" {
		t.Fatalf("due = %#v", due)
	}
	again, err := store.ClaimDue(now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClaimDue(again) error = %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("again = %#v", again)
	}
	deleted, err := store.Delete(first.ID)
	if err != nil || !deleted {
		t.Fatalf("Delete() = %v, %v", deleted, err)
	}
}
