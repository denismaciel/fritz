package secretstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const CurrentVersion = 1

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)

type Paths struct {
	Root string
	File string
}

type Entry struct {
	Name      string
	Value     string
	UpdatedAt string
}

type ListEntry struct {
	Name      string
	UpdatedAt string
}

type fileEntry struct {
	Value     string `json:"value"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type fileData struct {
	Version int                  `json:"version"`
	Secrets map[string]fileEntry `json:"secrets"`
}

type Store struct {
	path string
	now  func() time.Time
}

func ResolvePaths(cwd string) Paths {
	root := filepath.Join(cwd, ".fritz")
	return Paths{
		Root: root,
		File: filepath.Join(root, "secrets.json"),
	}
}

func New(cwd string) *Store {
	return &Store{
		path: ResolvePaths(cwd).File,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func ValidateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid secret name %q", name)
	}
	return nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Set(name string, value string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if value == "" {
		return errors.New("secret value must not be empty")
	}
	data, err := s.load()
	if err != nil {
		return err
	}
	data.Secrets[name] = fileEntry{
		Value:     value,
		UpdatedAt: s.now().Format(time.RFC3339),
	}
	return writeAtomic(s.path, data, 0o600)
}

func (s *Store) Get(name string) (Entry, bool, error) {
	if err := ValidateName(name); err != nil {
		return Entry{}, false, err
	}
	data, err := s.load()
	if err != nil {
		return Entry{}, false, err
	}
	entry, ok := data.Secrets[name]
	if !ok {
		return Entry{}, false, nil
	}
	return Entry{Name: name, Value: entry.Value, UpdatedAt: entry.UpdatedAt}, true, nil
}

func (s *Store) Delete(name string) (bool, error) {
	if err := ValidateName(name); err != nil {
		return false, err
	}
	data, err := s.load()
	if err != nil {
		return false, err
	}
	if _, ok := data.Secrets[name]; !ok {
		return false, nil
	}
	delete(data.Secrets, name)
	return true, writeAtomic(s.path, data, 0o600)
}

func (s *Store) List() ([]ListEntry, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(data.Secrets))
	for name := range data.Secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ListEntry, 0, len(names))
	for _, name := range names {
		out = append(out, ListEntry{Name: name, UpdatedAt: data.Secrets[name].UpdatedAt})
	}
	return out, nil
}

func (s *Store) load() (fileData, error) {
	data := fileData{
		Version: CurrentVersion,
		Secrets: map[string]fileEntry{},
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return data, nil
		}
		return fileData{}, err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fileData{}, err
	}
	if data.Secrets == nil {
		data.Secrets = map[string]fileEntry{}
	}
	return data, nil
}

func writeAtomic(path string, value any, mode os.FileMode) error {
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
	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
