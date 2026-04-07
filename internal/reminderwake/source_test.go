package reminderwake

import (
	"context"
	"testing"
	"time"

	"fritz/internal/config"
	"fritz/internal/gateway"
	"fritz/internal/reminder"
)

func TestSourceTurnsDueReminderIntoTelegramWake(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC)
	paths := gateway.ResolveStatePaths(dir, config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
	}))
	store := reminder.NewStoreAtPath(paths.ReminderPath)
	_, err := store.Set(now, reminder.ReminderInput{
		SessionPath: "/tmp/s1.jsonl",
		When:        now.Add(-time.Minute),
		Text:        "do the thing",
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	state := gateway.SessionMapFile{
		Version: gateway.CurrentStoreVersion,
		Sessions: map[string]string{
			"telegram:dm:7": "/tmp/s1.jsonl",
		},
	}
	if err := gateway.WriteJSONFileAtomic(paths.RoutingSessionMapPath, state); err != nil {
		t.Fatalf("WriteJSONFileAtomic() error = %v", err)
	}

	source := New(paths.ReminderPath, paths.RoutingSessionMapPath)
	wakes, err := source.Due(context.Background(), now)
	if err != nil {
		t.Fatalf("Due() error = %v", err)
	}
	if len(wakes) != 1 {
		t.Fatalf("wakes = %#v", wakes)
	}
	if wakes[0].Channel != "telegram" || wakes[0].ChatID != "7" || wakes[0].UserID != "7" {
		t.Fatalf("wake = %#v", wakes[0])
	}
}
