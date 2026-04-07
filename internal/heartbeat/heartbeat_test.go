package heartbeat

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestQueueCoalescesByTarget(t *testing.T) {
	q := NewQueue()
	if !q.Enqueue(Wake{TargetKey: "a", Reason: "one"}) {
		t.Fatal("first enqueue = false")
	}
	if q.Enqueue(Wake{TargetKey: "a", Reason: "two"}) {
		t.Fatal("second enqueue = true")
	}
	if !q.Enqueue(Wake{TargetKey: "b"}) {
		t.Fatal("third enqueue = false")
	}
	first, ok := q.Dequeue()
	if !ok || first.TargetKey != "a" {
		t.Fatalf("first = %#v %t", first, ok)
	}
	second, ok := q.Dequeue()
	if !ok || second.TargetKey != "b" {
		t.Fatalf("second = %#v %t", second, ok)
	}
}

func TestJSONStoreRoundTrip(t *testing.T) {
	store := NewJSONStoreAt(filepath.Join(t.TempDir(), "heartbeat.json"))
	want := State{
		Version:   1,
		LastTick:  "2026-04-03T00:00:00Z",
		LastRunAt: "2026-04-03T00:00:00Z",
		Pending:   []Wake{{TargetKey: "a"}},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.LastTick != want.LastTick || len(got.Pending) != 1 || got.Pending[0].TargetKey != "a" {
		t.Fatalf("got = %#v", got)
	}
}

func TestInterpretNoOp(t *testing.T) {
	if got := Interpret(" HEARTBEAT_OK "); got.Actionable {
		t.Fatalf("got = %#v", got)
	}
	if got := Interpret("work"); !got.Actionable || got.Text != "work" {
		t.Fatalf("got = %#v", got)
	}
}

func TestManagerTickSendsOnlyActionable(t *testing.T) {
	source := fakeSource{wakes: []Wake{{TargetKey: "a"}, {TargetKey: "b"}}}
	runner := &fakeRunner{
		results: map[string]Result{
			"a": Interpret(NoOpSentinel),
			"b": Interpret("send this"),
		},
	}
	sender := &fakeSender{}
	store := NewJSONStoreAt(filepath.Join(t.TempDir(), "heartbeat.json"))
	manager, err := NewManager(time.Minute, source, runner, sender, store)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	manager.now = func() time.Time { return time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC) }
	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0].Text != "send this" || sender.sent[0].Wake.TargetKey != "b" {
		t.Fatalf("sent = %#v", sender.sent)
	}
}

func TestManagerRequeuesOnSendFailure(t *testing.T) {
	source := fakeSource{wakes: []Wake{{TargetKey: "a"}}}
	runner := &fakeRunner{results: map[string]Result{"a": Interpret("send this")}}
	sender := &fakeSender{err: errors.New("boom")}
	store := NewJSONStoreAt(filepath.Join(t.TempDir(), "heartbeat.json"))
	manager, err := NewManager(time.Minute, source, runner, sender, store)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	err = manager.Tick(context.Background())
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Tick() err = %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Pending) != 1 || state.Pending[0].TargetKey != "a" {
		t.Fatalf("state = %#v", state)
	}
}

type fakeSource struct {
	wakes []Wake
	err   error
}

func (f fakeSource) Due(context.Context, time.Time) ([]Wake, error) {
	return append([]Wake(nil), f.wakes...), f.err
}

type fakeRunner struct {
	results map[string]Result
	err     error
}

func (f *fakeRunner) Run(_ context.Context, wake Wake) (Result, error) {
	if f.err != nil {
		return Result{}, f.err
	}
	return f.results[wake.TargetKey], nil
}

type sentMessage struct {
	Wake Wake
	Text string
}

type fakeSender struct {
	sent []sentMessage
	err  error
}

func (f *fakeSender) Send(_ context.Context, wake Wake, text string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sentMessage{Wake: wake, Text: text})
	return nil
}
