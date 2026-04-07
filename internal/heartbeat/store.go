package heartbeat

import (
	"path/filepath"

	"fritz/internal/gateway"
)

type JSONStore struct {
	path string
}

func NewJSONStore(root string) *JSONStore {
	return &JSONStore{path: filepath.Join(root, "heartbeat", "state.json")}
}

func NewJSONStoreAt(path string) *JSONStore {
	return &JSONStore{path: path}
}

func NewJSONStoreForPaths(paths gateway.StatePaths) *JSONStore {
	return &JSONStore{path: paths.HeartbeatStatePath}
}

func (s *JSONStore) Load() (State, error) {
	state, _, err := gateway.ReadJSONFile(s.path, State{})
	if err != nil {
		return State{}, err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Pending == nil {
		state.Pending = []Wake{}
	}
	return state, nil
}

func (s *JSONStore) Save(state State) error {
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Pending == nil {
		state.Pending = []Wake{}
	}
	return gateway.WriteJSONFileAtomic(s.path, state)
}
