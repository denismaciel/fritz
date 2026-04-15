package ingress

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func ReadJSONFile[T any](path string, fallback T) (T, bool, error) {
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

func WriteJSONFileAtomic(path string, value any) error {
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

type MetaFile struct {
	Version   int    `json:"version"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type SessionMapFile struct {
	Version  int               `json:"version"`
	Sessions map[string]string `json:"sessions"`
}

type BindingsFile struct {
	Version  int      `json:"version"`
	Bindings []string `json:"bindings"`
}

type TelegramOffsetFile struct {
	Version    int   `json:"version"`
	NextOffset int64 `json:"nextOffset"`
}

type TelegramAllowlistFile struct {
	Version int      `json:"version"`
	Users   []string `json:"users"`
}

type TelegramPairingRecord struct {
	UserID   string `json:"userId"`
	ChatID   string `json:"chatId,omitempty"`
	PairedAt string `json:"pairedAt"`
}

type TelegramPairingFile struct {
	Version int                     `json:"version"`
	Paired  []TelegramPairingRecord `json:"paired"`
}

type SlackAllowlistFile struct {
	Version  int      `json:"version"`
	Users    []string `json:"users,omitempty"`
	Channels []string `json:"channels,omitempty"`
}

type SlackEventStateFile struct {
	Version      int      `json:"version"`
	RecentEvents []string `json:"recentEvents,omitempty"`
}

type SlackContextBinding struct {
	RouteKey  string            `json:"routeKey"`
	TeamID    string            `json:"teamId,omitempty"`
	ChannelID string            `json:"channelId,omitempty"`
	ThreadTS  string            `json:"threadTs,omitempty"`
	Title     string            `json:"title,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	UpdatedAt string            `json:"updatedAt,omitempty"`
}

type SlackContextStateFile struct {
	Version  int                   `json:"version"`
	Bindings []SlackContextBinding `json:"bindings,omitempty"`
}
