package observe

import (
	"testing"

	"fritz/internal/engine"
)

func TestHubSubscribeReplaysBufferedEvents(t *testing.T) {
	hub := NewHub(10, 5)
	hub.StartRun("run-1", "hello")
	hub.Publish(engine.Event{RunID: "run-1", Kind: engine.EventRunStarted})
	hub.Publish(engine.Event{RunID: "run-1", Kind: engine.EventRunFinished})
	hub.FinishSubscribers("run-1")

	events, unsubscribe, ok := hub.Subscribe("run-1")
	defer unsubscribe()
	if !ok {
		t.Fatal("Subscribe() ok = false")
	}

	var kinds []engine.EventKind
	for event := range events {
		kinds = append(kinds, event.Kind)
	}
	if len(kinds) != 2 {
		t.Fatalf("got %d events, want 2", len(kinds))
	}
	if kinds[0] != engine.EventRunStarted || kinds[1] != engine.EventRunFinished {
		t.Fatalf("kinds = %#v", kinds)
	}
}

func TestHubPublishDoesNotBlockOnFullSubscriber(t *testing.T) {
	hub := NewHub(10, 5)
	hub.StartRun("run-1", "hello")
	events, unsubscribe, ok := hub.Subscribe("run-1")
	defer unsubscribe()
	if !ok {
		t.Fatal("Subscribe() ok = false")
	}
	for i := 0; i < cap(events)+10; i++ {
		hub.Publish(engine.Event{RunID: "run-1", Kind: engine.EventTextDelta})
	}
}
