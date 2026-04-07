package logx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.jsonl")
	closeFn, err := Configure(Config{Path: path, Level: "debug"})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	defer func() { _ = closeFn() }()

	logger := Component("test")
	logger.Info().Str("event", "hello").Int("n", 1).Msg("")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if row["service"] != "fritz" || row["component"] != "test" || row["event"] != "hello" {
		t.Fatalf("row = %#v", row)
	}
}

func TestWithContextOverridesBase(t *testing.T) {
	ctx := WithContext(context.Background(), Component("ctx").With().Str("event", "ctx").Logger())
	logger := FromContext(ctx)
	if logger.GetLevel() != Base().GetLevel() {
		t.Fatalf("logger level = %v", logger.GetLevel())
	}
}
