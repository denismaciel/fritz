package heartbeat

import (
	"context"
	"strings"
	"sync"
	"time"

	"fritz/internal/logx"
)

const NoOpSentinel = "HEARTBEAT_OK"

type Wake struct {
	TargetKey string `json:"targetKey"`
	Channel   string `json:"channel,omitempty"`
	ChatID    string `json:"chatId,omitempty"`
	UserID    string `json:"userId,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func (w Wake) Key() string {
	return strings.TrimSpace(w.TargetKey)
}

type Queue struct {
	mu    sync.Mutex
	order []string
	items map[string]Wake
}

func NewQueue() *Queue {
	return &Queue{items: map[string]Wake{}}
}

func (q *Queue) Enqueue(w Wake) bool {
	key := w.Key()
	if key == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.items[key]; ok {
		return false
	}
	q.items[key] = w
	q.order = append(q.order, key)
	return true
}

func (q *Queue) Dequeue() (Wake, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.order) == 0 {
		return Wake{}, false
	}
	key := q.order[0]
	q.order = q.order[1:]
	item := q.items[key]
	delete(q.items, key)
	return item, true
}

func (q *Queue) Snapshot() []Wake {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Wake, 0, len(q.order))
	for _, key := range q.order {
		out = append(out, q.items[key])
	}
	return out
}

func (q *Queue) Restore(items []Wake) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.order = nil
	q.items = map[string]Wake{}
	for _, item := range items {
		key := item.Key()
		if key == "" {
			continue
		}
		if _, ok := q.items[key]; ok {
			continue
		}
		q.items[key] = item
		q.order = append(q.order, key)
	}
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.order)
}

type Source interface {
	Due(context.Context, time.Time) ([]Wake, error)
}

type NullSource struct{}

func (NullSource) Due(context.Context, time.Time) ([]Wake, error) {
	return nil, nil
}

type MultiSource struct {
	Sources []Source
}

func CombineSources(sources ...Source) Source {
	filtered := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		filtered = append(filtered, source)
	}
	if len(filtered) == 0 {
		return NullSource{}
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return MultiSource{Sources: filtered}
}

func (m MultiSource) Due(ctx context.Context, now time.Time) ([]Wake, error) {
	var out []Wake
	for _, source := range m.Sources {
		items, err := source.Due(ctx, now)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

type Result struct {
	Text       string
	Actionable bool
}

type Runner interface {
	Run(context.Context, Wake) (Result, error)
}

type Sender interface {
	Send(context.Context, Wake, string) error
}

type State struct {
	Version   int    `json:"version"`
	LastTick  string `json:"lastTick,omitempty"`
	LastRunAt string `json:"lastRunAt,omitempty"`
	Pending   []Wake `json:"pending,omitempty"`
}

type Store interface {
	Load() (State, error)
	Save(State) error
}

type Manager struct {
	interval time.Duration
	queue    *Queue
	source   Source
	runner   Runner
	sender   Sender
	store    Store
	now      func() time.Time
}

func NewManager(interval time.Duration, source Source, runner Runner, sender Sender, store Store) (*Manager, error) {
	if interval <= 0 {
		interval = time.Minute
	}
	if source == nil {
		source = NullSource{}
	}
	manager := &Manager{
		interval: interval,
		queue:    NewQueue(),
		source:   source,
		runner:   runner,
		sender:   sender,
		store:    store,
		now:      func() time.Time { return time.Now().UTC() },
	}
	if store != nil {
		state, err := store.Load()
		if err != nil {
			return nil, err
		}
		manager.queue.Restore(state.Pending)
	}
	return manager, nil
}

func (m *Manager) Enqueue(w Wake) bool {
	added := m.queue.Enqueue(w)
	if added {
		logger := logx.Component("heartbeat")
		logger.Info().Str("event", "heartbeat.queue.enqueue").Str("target_key", w.TargetKey).Msg("")
		_ = m.persist("")
	}
	return added
}

func (m *Manager) Tick(ctx context.Context) error {
	now := m.now().UTC()
	logger := logx.Component("heartbeat").With().Str("event", "heartbeat.tick").Str("now", now.Format(time.RFC3339Nano)).Logger()
	due, err := m.source.Due(ctx, now)
	if err != nil {
		logger.Error().Err(err).Str("stage", "source.due").Msg("")
		return err
	}
	logger.Info().Int("due", len(due)).Int("queued", m.queue.Len()).Msg("")
	for _, wake := range due {
		m.queue.Enqueue(wake)
	}
	if err := m.persist(now.Format(time.RFC3339Nano)); err != nil {
		logger.Error().Err(err).Str("stage", "persist.before").Msg("")
		return err
	}
	for {
		wake, ok := m.queue.Dequeue()
		if !ok {
			logger.Info().Str("stage", "done").Int("queued", m.queue.Len()).Msg("")
			return m.persist(now.Format(time.RFC3339Nano))
		}
		logger.Info().Str("stage", "run.start").Str("target_key", wake.TargetKey).Msg("")
		result, err := m.runner.Run(ctx, wake)
		if err != nil {
			_ = m.queue.Enqueue(wake)
			_ = m.persist(now.Format(time.RFC3339Nano))
			logger.Error().Err(err).Str("stage", "run.error").Str("target_key", wake.TargetKey).Msg("")
			return err
		}
		logger.Info().Str("stage", "run.finish").Str("target_key", wake.TargetKey).Bool("actionable", result.Actionable).Int("text_len", len(strings.TrimSpace(result.Text))).Msg("")
		if result.Actionable && strings.TrimSpace(result.Text) != "" {
			if err := m.sender.Send(ctx, wake, result.Text); err != nil {
				_ = m.queue.Enqueue(wake)
				_ = m.persist(now.Format(time.RFC3339Nano))
				logger.Error().Err(err).Str("stage", "send.error").Str("target_key", wake.TargetKey).Msg("")
				return err
			}
		}
		if err := m.persist(now.Format(time.RFC3339Nano)); err != nil {
			logger.Error().Err(err).Str("stage", "persist.after").Str("target_key", wake.TargetKey).Msg("")
			return err
		}
	}
}

func (m *Manager) Run(ctx context.Context) error {
	root := logx.Component("heartbeat")
	root.Info().Str("event", "heartbeat.run.start").Dur("interval", m.interval).Msg("")
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			root.Info().Str("event", "heartbeat.run.stop").Msg("")
			return nil
		case <-ticker.C:
			if err := m.Tick(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				root.Error().Err(err).Str("event", "heartbeat.run.error").Msg("")
				return err
			}
		}
	}
}

func (m *Manager) persist(lastTick string) error {
	if m.store == nil {
		return nil
	}
	state, err := m.store.Load()
	if err != nil {
		return err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if lastTick != "" {
		state.LastTick = lastTick
		state.LastRunAt = lastTick
	}
	state.Pending = m.queue.Snapshot()
	return m.store.Save(state)
}

func Interpret(text string) Result {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || trimmed == NoOpSentinel {
		return Result{Text: "", Actionable: false}
	}
	return Result{Text: trimmed, Actionable: true}
}
