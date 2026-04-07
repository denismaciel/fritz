package logx

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

type Config struct {
	Path  string
	Level string
}

type contextKey struct{}

type state struct {
	mu     sync.RWMutex
	logger zerolog.Logger
	file   io.Closer
	path   string
}

var global = &state{logger: zerolog.Nop()}

func Configure(cfg Config) (func() error, error) {
	logger := zerolog.Nop()
	var file io.Closer
	path := strings.TrimSpace(cfg.Path)
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		handle, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		level, err := parseLevel(cfg.Level)
		if err != nil {
			_ = handle.Close()
			return nil, err
		}
		logger = zerolog.New(handle).
			Level(level).
			With().
			Timestamp().
			Str("service", "fritz").
			Logger()
		file = handle
	}

	global.mu.Lock()
	old := global.file
	global.file = file
	global.logger = logger
	global.path = path
	global.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}

	return func() error {
		global.mu.Lock()
		defer global.mu.Unlock()
		if global.file == nil {
			return nil
		}
		err := global.file.Close()
		global.file = nil
		global.logger = zerolog.Nop()
		global.path = ""
		return err
	}, nil
}

func Path() string {
	global.mu.RLock()
	defer global.mu.RUnlock()
	return global.path
}

func Base() zerolog.Logger {
	global.mu.RLock()
	defer global.mu.RUnlock()
	return global.logger
}

func Component(name string) zerolog.Logger {
	return Base().With().Str("component", name).Logger()
}

func WithContext(ctx context.Context, logger zerolog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

func FromContext(ctx context.Context) zerolog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(contextKey{}).(zerolog.Logger); ok {
			return logger
		}
	}
	return Base()
}

func parseLevel(raw string) (zerolog.Level, error) {
	level := strings.TrimSpace(raw)
	if level == "" {
		level = "info"
	}
	return zerolog.ParseLevel(strings.ToLower(level))
}
