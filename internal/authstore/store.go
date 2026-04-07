package authstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"fritz/internal/provider"
)

const CurrentVersion = 1

type Paths struct {
	Root     string
	File     string
	LockFile string
}

type OAuthCredential struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	AccountID    string    `json:"accountId"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type Credential struct {
	Provider  provider.Kind    `json:"provider"`
	APIKey    string           `json:"apiKey,omitempty"`
	OAuth     *OAuthCredential `json:"oauth,omitempty"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type ListEntry struct {
	Provider  provider.Kind
	Kind      string
	UpdatedAt time.Time
}

type Store interface {
	Get(kind provider.Kind) (Credential, bool, error)
	PutAPIKey(kind provider.Kind, value string) error
	PutOAuth(kind provider.Kind, value OAuthCredential) error
	Delete(kind provider.Kind) (bool, error)
	List() ([]ListEntry, error)
	Update(kind provider.Kind, fn func(Credential, bool) (Credential, bool, error)) error
}

type FileStore struct {
	path     string
	lockPath string
	now      func() time.Time
}

type MemoryStore struct {
	mu   sync.Mutex
	data map[provider.Kind]Credential
	now  func() time.Time
}

type fileData struct {
	Version     int                          `json:"version"`
	Credentials map[provider.Kind]Credential `json:"credentials"`
}

func GlobalAuthPath() string {
	if path := os.Getenv("XDG_DATA_HOME"); path != "" {
		return filepath.Join(path, "fritz", "auth.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "fritz", "auth.json")
}

func GlobalAuthLockPath() string {
	path := GlobalAuthPath()
	if path == "" {
		return ""
	}
	return path + ".lock"
}

func ResolvePaths(cwd string) Paths {
	root := filepath.Join(cwd, ".fritz")
	return Paths{
		Root:     root,
		File:     filepath.Join(root, "auth.json"),
		LockFile: filepath.Join(root, "auth.lock"),
	}
}

func NewFileStore(cwd string) *FileStore {
	paths := ResolvePaths(cwd)
	return &FileStore{
		path:     paths.File,
		lockPath: paths.LockFile,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func NewGlobalFileStore() *FileStore {
	return &FileStore{
		path:     GlobalAuthPath(),
		lockPath: GlobalAuthLockPath(),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: map[provider.Kind]Credential{},
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *FileStore) Path() string {
	return s.path
}

func (s *FileStore) Get(kind provider.Kind) (Credential, bool, error) {
	var out Credential
	var ok bool
	err := s.withLock(func() error {
		data, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		out, ok = data.Credentials[kind]
		return nil
	})
	return out, ok, err
}

func (s *FileStore) PutAPIKey(kind provider.Kind, value string) error {
	if value == "" {
		return errors.New("api key must not be empty")
	}
	return s.Update(kind, func(_ Credential, _ bool) (Credential, bool, error) {
		return Credential{
			Provider:  kind,
			APIKey:    value,
			UpdatedAt: s.now(),
		}, true, nil
	})
}

func (s *FileStore) PutOAuth(kind provider.Kind, value OAuthCredential) error {
	if err := validateOAuth(value); err != nil {
		return err
	}
	return s.Update(kind, func(_ Credential, _ bool) (Credential, bool, error) {
		return Credential{
			Provider:  kind,
			OAuth:     &value,
			UpdatedAt: s.now(),
		}, true, nil
	})
}

func (s *FileStore) Delete(kind provider.Kind) (bool, error) {
	var deleted bool
	err := s.Update(kind, func(_ Credential, ok bool) (Credential, bool, error) {
		deleted = ok
		return Credential{}, false, nil
	})
	return deleted, err
}

func (s *FileStore) List() ([]ListEntry, error) {
	var out []ListEntry
	err := s.withLock(func() error {
		data, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		out = listEntries(data.Credentials)
		return nil
	})
	return out, err
}

func (s *FileStore) Update(kind provider.Kind, fn func(Credential, bool) (Credential, bool, error)) error {
	return s.withLock(func() error {
		data, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		current, ok := data.Credentials[kind]
		next, keep, err := fn(current, ok)
		if err != nil {
			return err
		}
		if !keep {
			delete(data.Credentials, kind)
		} else {
			next.Provider = kind
			if next.UpdatedAt.IsZero() {
				next.UpdatedAt = s.now()
			}
			data.Credentials[kind] = next
		}
		return secretstoreWriteAtomic(s.path, data, 0o600)
	})
}

func (s *FileStore) withLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.lockPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func (s *FileStore) loadUnlocked() (fileData, error) {
	data := fileData{
		Version:     CurrentVersion,
		Credentials: map[provider.Kind]Credential{},
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
	if data.Credentials == nil {
		data.Credentials = map[provider.Kind]Credential{}
	}
	return data, nil
}

func (s *MemoryStore) Get(kind provider.Kind) (Credential, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[kind]
	return value, ok, nil
}

func (s *MemoryStore) PutAPIKey(kind provider.Kind, value string) error {
	if value == "" {
		return errors.New("api key must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[kind] = Credential{
		Provider:  kind,
		APIKey:    value,
		UpdatedAt: s.now(),
	}
	return nil
}

func (s *MemoryStore) PutOAuth(kind provider.Kind, value OAuthCredential) error {
	if err := validateOAuth(value); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copyValue := value
	s.data[kind] = Credential{
		Provider:  kind,
		OAuth:     &copyValue,
		UpdatedAt: s.now(),
	}
	return nil
}

func (s *MemoryStore) Delete(kind provider.Kind) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[kind]
	delete(s.data, kind)
	return ok, nil
}

func (s *MemoryStore) List() ([]ListEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return listEntries(s.data), nil
}

func (s *MemoryStore) Update(kind provider.Kind, fn func(Credential, bool) (Credential, bool, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.data[kind]
	next, keep, err := fn(current, ok)
	if err != nil {
		return err
	}
	if !keep {
		delete(s.data, kind)
		return nil
	}
	next.Provider = kind
	if next.UpdatedAt.IsZero() {
		next.UpdatedAt = s.now()
	}
	s.data[kind] = next
	return nil
}

func (c Credential) Kind() string {
	switch {
	case c.OAuth != nil:
		return "oauth"
	case c.APIKey != "":
		return "api_key"
	default:
		return "unknown"
	}
}

func validateOAuth(value OAuthCredential) error {
	switch {
	case value.AccessToken == "":
		return errors.New("oauth access token must not be empty")
	case value.RefreshToken == "":
		return errors.New("oauth refresh token must not be empty")
	case value.AccountID == "":
		return errors.New("oauth account id must not be empty")
	case value.ExpiresAt.IsZero():
		return errors.New("oauth expiry must not be zero")
	default:
		return nil
	}
}

func listEntries(values map[provider.Kind]Credential) []ListEntry {
	keys := make([]string, 0, len(values))
	for kind := range values {
		keys = append(keys, string(kind))
	}
	sort.Strings(keys)
	out := make([]ListEntry, 0, len(keys))
	for _, key := range keys {
		value := values[provider.Kind(key)]
		out = append(out, ListEntry{
			Provider:  value.Provider,
			Kind:      value.Kind(),
			UpdatedAt: value.UpdatedAt,
		})
	}
	return out
}

func secretstoreWriteAtomic(path string, value any, mode os.FileMode) error {
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

func FormatStatus(entry Credential) string {
	switch {
	case entry.OAuth != nil:
		return fmt.Sprintf("oauth expires=%s", entry.OAuth.ExpiresAt.UTC().Format(time.RFC3339))
	case entry.APIKey != "":
		return "api_key"
	default:
		return "unknown"
	}
}
