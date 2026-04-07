package reminderwake

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fritz/internal/heartbeat"
	"fritz/internal/reminder"
)

type Source struct {
	store          *reminder.Store
	sessionMapPath string
}

type sessionMapFile struct {
	Version  int               `json:"version"`
	Sessions map[string]string `json:"sessions"`
}

func New(reminderPath string, sessionMapPath string) *Source {
	return &Source{
		store:          reminder.NewStoreAtPath(reminderPath),
		sessionMapPath: sessionMapPath,
	}
}

func (s *Source) Due(_ context.Context, now time.Time) ([]heartbeat.Wake, error) {
	items, err := s.store.ClaimDue(now)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	state, _, err := readJSONFile(s.sessionMapPath, sessionMapFile{})
	if err != nil {
		return nil, err
	}
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	byPath := map[string]string{}
	for key, path := range state.Sessions {
		if strings.TrimSpace(path) != "" {
			byPath[strings.TrimSpace(path)] = key
		}
	}
	var wakes []heartbeat.Wake
	for _, item := range items {
		key := byPath[strings.TrimSpace(item.SessionPath)]
		if key == "" {
			continue
		}
		wake, ok := wakeFromSessionKey(key)
		if !ok {
			continue
		}
		wake.Reason = fmt.Sprintf("Reminder due at %s:\n%s", item.When, item.Text)
		wakes = append(wakes, wake)
	}
	return wakes, nil
}

func wakeFromSessionKey(key string) (heartbeat.Wake, bool) {
	parts := strings.Split(strings.TrimSpace(key), ":")
	if len(parts) != 3 {
		return heartbeat.Wake{}, false
	}
	wake := heartbeat.Wake{
		TargetKey: key,
		Channel:   parts[0],
	}
	switch parts[1] {
	case "dm":
		wake.ChatID = parts[2]
		wake.UserID = parts[2]
	case "group":
		wake.ChatID = parts[2]
	default:
		wake.ChatID = parts[2]
	}
	return wake, true
}
