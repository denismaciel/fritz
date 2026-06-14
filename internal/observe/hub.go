package observe

import (
	"sort"
	"strings"
	"sync"
	"time"

	"fritz/internal/engine"
)

type Status string

const (
	StatusRunning  Status = "running"
	StatusFinished Status = "finished"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)

type RunInfo struct {
	ID          string            `json:"id"`
	Status      Status            `json:"status"`
	Session     engine.SessionRef `json:"session,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	StartedAt   time.Time         `json:"startedAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	CompletedAt time.Time         `json:"completedAt,omitempty"`
}

type Hub struct {
	mu          sync.Mutex
	bufferLimit int
	keepRuns    int
	runs        map[string]*runRecord
	order       []string
}

type runRecord struct {
	info        RunInfo
	events      []engine.Event
	subscribers map[chan engine.Event]struct{}
}

func NewHub(bufferLimit int, keepRuns int) *Hub {
	if bufferLimit <= 0 {
		bufferLimit = 512
	}
	if keepRuns <= 0 {
		keepRuns = 20
	}
	return &Hub{
		bufferLimit: bufferLimit,
		keepRuns:    keepRuns,
		runs:        map[string]*runRecord{},
	}
}

func (h *Hub) StartRun(id string, prompt string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.runs[id]; ok {
		return
	}
	h.runs[id] = &runRecord{
		info: RunInfo{
			ID:        id,
			Status:    StatusRunning,
			Prompt:    promptSnippet(prompt),
			StartedAt: now,
			UpdatedAt: now,
		},
		subscribers: map[chan engine.Event]struct{}{},
	}
	h.order = append(h.order, id)
	h.pruneLocked()
}

func (h *Hub) Publish(event engine.Event) {
	runID := strings.TrimSpace(event.RunID)
	if runID == "" {
		return
	}
	h.mu.Lock()
	record, ok := h.runs[runID]
	if !ok {
		now := time.Now().UTC()
		record = &runRecord{
			info: RunInfo{
				ID:        runID,
				Status:    StatusRunning,
				StartedAt: now,
				UpdatedAt: now,
			},
			subscribers: map[chan engine.Event]struct{}{},
		}
		h.runs[runID] = record
		h.order = append(h.order, runID)
	}
	record.info.UpdatedAt = time.Now().UTC()
	if event.Session.ID != "" || event.Session.Path != "" {
		record.info.Session = event.Session
	}
	switch event.Kind {
	case engine.EventRunFinished:
		record.info.Status = StatusFinished
		record.info.CompletedAt = record.info.UpdatedAt
	case engine.EventRunFailed:
		record.info.Status = StatusFailed
		record.info.CompletedAt = record.info.UpdatedAt
	case engine.EventRunCanceled:
		record.info.Status = StatusCanceled
		record.info.CompletedAt = record.info.UpdatedAt
	}
	record.events = append(record.events, event)
	if len(record.events) > h.bufferLimit {
		record.events = append([]engine.Event(nil), record.events[len(record.events)-h.bufferLimit:]...)
	}
	subscribers := make([]chan engine.Event, 0, len(record.subscribers))
	for ch := range record.subscribers {
		subscribers = append(subscribers, ch)
	}
	h.pruneLocked()
	h.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (h *Hub) ListRuns() []RunInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]RunInfo, 0, len(h.runs))
	for _, record := range h.runs {
		out = append(out, record.info)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (h *Hub) Subscribe(runID string) (<-chan engine.Event, func(), bool) {
	runID = strings.TrimSpace(runID)
	h.mu.Lock()
	record, ok := h.runs[runID]
	if !ok {
		ch := make(chan engine.Event)
		h.mu.Unlock()
		close(ch)
		return ch, func() {}, false
	}
	replay := append([]engine.Event(nil), record.events...)
	live := record.info.Status == StatusRunning
	ch := make(chan engine.Event, len(replay)+128)
	for _, event := range replay {
		ch <- event
	}
	if live {
		record.subscribers[ch] = struct{}{}
	}
	h.mu.Unlock()

	if !live {
		close(ch)
	}

	unsubscribe := func() {
		h.mu.Lock()
		if record, ok := h.runs[runID]; ok {
			delete(record.subscribers, ch)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe, true
}

func (h *Hub) FinishSubscribers(runID string) {
	h.mu.Lock()
	record, ok := h.runs[strings.TrimSpace(runID)]
	if !ok {
		h.mu.Unlock()
		return
	}
	subscribers := make([]chan engine.Event, 0, len(record.subscribers))
	for ch := range record.subscribers {
		subscribers = append(subscribers, ch)
		delete(record.subscribers, ch)
	}
	h.mu.Unlock()
	for _, ch := range subscribers {
		close(ch)
	}
}

func (h *Hub) pruneLocked() {
	if len(h.order) <= h.keepRuns {
		return
	}
	kept := h.order[:0]
	for _, id := range h.order {
		record := h.runs[id]
		if record == nil {
			continue
		}
		if record.info.Status == StatusRunning {
			kept = append(kept, id)
			continue
		}
		if len(h.order)-len(kept) <= h.keepRuns {
			kept = append(kept, id)
			continue
		}
		delete(h.runs, id)
	}
	h.order = kept
}

func promptSnippet(prompt string) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if len(prompt) <= 120 {
		return prompt
	}
	return prompt[:117] + "..."
}
