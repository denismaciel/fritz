package reminderwake

import (
	"encoding/json"
	"errors"
	"os"
)

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
