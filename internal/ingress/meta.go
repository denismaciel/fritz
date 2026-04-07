package ingress

import "time"

func EnsureLayout(paths StatePaths, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	meta, _, err := ReadJSONFile(paths.MetaPath, MetaFile{})
	if err != nil {
		return err
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	if meta.Version == 0 {
		meta.Version = CurrentStoreVersion
		meta.CreatedAt = timestamp
	}
	meta.UpdatedAt = timestamp
	if err := WriteJSONFileAtomic(paths.MetaPath, meta); err != nil {
		return err
	}
	bindings, exists, err := ReadJSONFile(paths.BindingsCurrentPath, BindingsFile{})
	if err != nil {
		return err
	}
	if !exists {
		bindings = BindingsFile{Version: CurrentStoreVersion, Bindings: []string{}}
		return WriteJSONFileAtomic(paths.BindingsCurrentPath, bindings)
	}
	if bindings.Version == 0 {
		bindings.Version = CurrentStoreVersion
	}
	return WriteJSONFileAtomic(paths.BindingsCurrentPath, bindings)
}
