package reminder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const CurrentVersion = 1

type Reminder struct {
	ID          string `json:"id"`
	SessionPath string `json:"sessionPath"`
	When        string `json:"when"`
	Text        string `json:"text"`
	CreatedAt   string `json:"createdAt"`
	TriggeredAt string `json:"triggeredAt,omitempty"`
}

type File struct {
	Version   int        `json:"version"`
	Reminders []Reminder `json:"reminders"`
}

type ReminderInput struct {
	SessionPath string
	When        time.Time
	Text        string
}

type Store struct {
	path string
}

func NewStore(cwd string) *Store {
	return &Store{path: filepath.Join(cwd, ".fritz", "gateway", "reminders", "reminders.json")}
}

func NewStoreAt(path string) *Store {
	return &Store{path: path}
}

func NewStoreAtPath(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (File, error) {
	state, _, err := readJSONFile(s.path, File{})
	if err != nil {
		return File{}, err
	}
	if state.Version == 0 {
		state.Version = CurrentVersion
	}
	if state.Reminders == nil {
		state.Reminders = []Reminder{}
	}
	return state, nil
}

func (s *Store) Save(state File) error {
	if state.Version == 0 {
		state.Version = CurrentVersion
	}
	if state.Reminders == nil {
		state.Reminders = []Reminder{}
	}
	return writeJSONFileAtomic(s.path, state)
}

func (s *Store) Set(now time.Time, input ReminderInput) (Reminder, error) {
	if strings.TrimSpace(input.SessionPath) == "" {
		return Reminder{}, fmt.Errorf("reminder requires current session")
	}
	if strings.TrimSpace(input.Text) == "" {
		return Reminder{}, fmt.Errorf("reminder text missing")
	}
	if input.When.IsZero() {
		return Reminder{}, fmt.Errorf("reminder time missing")
	}
	state, err := s.Load()
	if err != nil {
		return Reminder{}, err
	}
	item := Reminder{
		ID:          newID(),
		SessionPath: strings.TrimSpace(input.SessionPath),
		When:        input.When.UTC().Format(time.RFC3339Nano),
		Text:        strings.TrimSpace(input.Text),
		CreatedAt:   now.UTC().Format(time.RFC3339Nano),
	}
	state.Reminders = append(state.Reminders, item)
	sort.Slice(state.Reminders, func(i, j int) bool { return state.Reminders[i].When < state.Reminders[j].When })
	if err := s.Save(state); err != nil {
		return Reminder{}, err
	}
	return item, nil
}

func (s *Store) List() ([]Reminder, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := append([]Reminder(nil), state.Reminders...)
	sort.Slice(out, func(i, j int) bool { return out[i].When < out[j].When })
	return out, nil
}

func (s *Store) Delete(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	state, err := s.Load()
	if err != nil {
		return false, err
	}
	filtered := state.Reminders[:0]
	deleted := false
	for _, item := range state.Reminders {
		if item.ID == id {
			deleted = true
			continue
		}
		filtered = append(filtered, item)
	}
	state.Reminders = filtered
	if !deleted {
		return false, nil
	}
	return true, s.Save(state)
}

func (s *Store) ClaimDue(now time.Time) ([]Reminder, error) {
	state, err := s.Load()
	if err != nil {
		return nil, err
	}
	var due []Reminder
	changed := false
	for i := range state.Reminders {
		item := &state.Reminders[i]
		if strings.TrimSpace(item.TriggeredAt) != "" {
			continue
		}
		when, err := time.Parse(time.RFC3339Nano, item.When)
		if err != nil {
			continue
		}
		if when.After(now.UTC()) {
			continue
		}
		item.TriggeredAt = now.UTC().Format(time.RFC3339Nano)
		due = append(due, *item)
		changed = true
	}
	if changed {
		if err := s.Save(state); err != nil {
			return nil, err
		}
	}
	return due, nil
}

func readJSONFile[T any](path string, fallback T) (T, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fallback, false, nil
		}
		return fallback, false, err
	}
	value := fallback
	if err := json.Unmarshal(data, &value); err != nil {
		return fallback, true, err
	}
	return value, true, nil
}

func writeJSONFileAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
