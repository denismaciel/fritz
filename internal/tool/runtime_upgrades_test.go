package tool

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type memoryFileInfo struct {
	name  string
	isDir bool
}

func (i memoryFileInfo) Name() string { return i.name }
func (i memoryFileInfo) Size() int64  { return 0 }
func (i memoryFileInfo) Mode() os.FileMode {
	if i.isDir {
		return os.ModeDir | 0o755
	}
	return 0o644
}
func (i memoryFileInfo) ModTime() time.Time { return time.Time{} }
func (i memoryFileInfo) IsDir() bool        { return i.isDir }
func (i memoryFileInfo) Sys() any           { return nil }

type memoryFileOps struct {
	files map[string]string
	dirs  map[string]bool
}

func newMemoryFileOps() *memoryFileOps {
	return &memoryFileOps{
		files: map[string]string{},
		dirs:  map[string]bool{},
	}
}

func (m *memoryFileOps) Stat(name string) (os.FileInfo, error) {
	if m.dirs[name] {
		return memoryFileInfo{name: filepath.Base(name), isDir: true}, nil
	}
	if _, ok := m.files[name]; ok {
		return memoryFileInfo{name: filepath.Base(name)}, nil
	}
	return nil, os.ErrNotExist
}

func (m *memoryFileOps) ReadFile(name string) ([]byte, error) {
	value, ok := m.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(value), nil
}

func (m *memoryFileOps) WriteFile(name string, data []byte, _ os.FileMode) error {
	m.files[name] = string(data)
	return nil
}

func (m *memoryFileOps) MkdirAll(path string, _ os.FileMode) error {
	m.dirs[path] = true
	return nil
}

func (m *memoryFileOps) ReadDir(string) ([]os.DirEntry, error) {
	return nil, errors.New("not implemented")
}

func (m *memoryFileOps) WalkDir(string, fs.WalkDirFunc) error {
	return errors.New("not implemented")
}

func (m *memoryFileOps) CreateTemp(string, string) (*os.File, error) {
	return nil, errors.New("not implemented")
}

func TestInjectedFileOperations(t *testing.T) {
	t.Run("read uses injected ops", func(t *testing.T) {
		ops := newMemoryFileOps()
		root := "/repo"
		path := filepath.Join(root, "note.txt")
		ops.files[path] = "hello from memory"

		result, err := NewReadTool(root, DefaultReadMaxBytes, WithReadFileOperations(ops)).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "read",
			Args: map[string]any{"path": "note.txt"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.Text() != "hello from memory" {
			t.Fatalf("text = %q", result.Text())
		}
	})

	t.Run("write uses injected ops", func(t *testing.T) {
		ops := newMemoryFileOps()
		root := "/repo"

		_, err := NewWriteTool(root, WithWriteFileOperations(ops)).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "write",
			Args: map[string]any{"path": "nested/out.txt", "content": "memory write"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := ops.files[filepath.Join(root, "nested/out.txt")]; got != "memory write" {
			t.Fatalf("file = %q", got)
		}
		if !ops.dirs[filepath.Join(root, "nested")] {
			t.Fatal("expected parent dir creation")
		}
	})

	t.Run("edit uses injected ops", func(t *testing.T) {
		ops := newMemoryFileOps()
		root := "/repo"
		path := filepath.Join(root, "edit.txt")
		ops.files[path] = "before old after"

		_, err := NewEditTool(root, DefaultReadMaxBytes, WithEditFileOperations(ops)).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "edit",
			Args: map[string]any{"path": "edit.txt", "oldText": "old", "newText": "new"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := ops.files[path]; got != "before new after" {
			t.Fatalf("file = %q", got)
		}
	})
}

func TestDiscoveryHardening(t *testing.T) {
	t.Run("find respects nested gitignore negation", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.txt\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		nested := filepath.Join(dir, "nested")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(nested, ".gitignore"), []byte("!keep.txt\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "drop.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(nested, "keep.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		result, err := NewFindTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "find",
			Args: map[string]any{"pattern": "**/*.txt"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if strings.Contains(result.Text(), "drop.txt") {
			t.Fatalf("unexpected ignored file: %q", result.Text())
		}
		if !strings.Contains(result.Text(), "nested/keep.txt") {
			t.Fatalf("missing unignored file: %q", result.Text())
		}
	})

	t.Run("go grep supports glob filter", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("needle\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("needle\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		result, err := NewGrepTool(dir, WithGrepBackend(GrepBackendGo)).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "grep",
			Args: map[string]any{"pattern": "needle", "glob": "*.md"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "a.md") || strings.Contains(result.Text(), "b.txt") {
			t.Fatalf("unexpected grep output: %q", result.Text())
		}
	})

	t.Run("ripgrep backend fails when rg is unavailable", func(t *testing.T) {
		dir := t.TempDir()
		result, err := NewGrepTool(dir, WithRipgrepPath(filepath.Join(dir, "missing-rg"))).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "grep",
			Args: map[string]any{"pattern": "needle"},
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !result.IsError || !strings.Contains(result.Text(), "Failed to run ripgrep") {
			t.Fatalf("result = %#v text = %q", result, result.Text())
		}
	})

	t.Run("ripgrep backend matches pi limit notice", func(t *testing.T) {
		if _, err := exec.LookPath("rg"); err != nil {
			t.Skip("rg not available")
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "ctx.txt"), []byte("before\nmatch one\nafter\nmatch two\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := NewGrepTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "grep",
			Args: map[string]any{"pattern": "match", "context": 1, "limit": 1},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "ctx.txt-1- before") || !strings.Contains(result.Text(), "ctx.txt:2: match one") {
			t.Fatalf("missing context output: %q", result.Text())
		}
		if !strings.Contains(result.Text(), "[1 matches limit reached. Use limit=2 for more, or refine pattern]") {
			t.Fatalf("missing pi limit notice: %q", result.Text())
		}
		if strings.Contains(result.Text(), "match two") {
			t.Fatalf("unexpected second match: %q", result.Text())
		}
	})
}

func TestBashRuntimeUpgrades(t *testing.T) {
	t.Run("bash spills large output to file", func(t *testing.T) {
		dir := t.TempDir()
		result, err := NewBashTool(dir, WithOutputMaxBytes(32)).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "bash",
			Args: map[string]any{"command": "yes x | head -n 200"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		details, ok := result.Details.(BashResultDetails)
		if !ok || !details.Truncated || details.FullOutputPath == "" {
			t.Fatalf("details = %#v", result.Details)
		}
		if _, statErr := os.Stat(details.FullOutputPath); statErr != nil {
			t.Fatalf("Stat() error = %v", statErr)
		}
		if strings.HasPrefix(details.FullOutputPath, dir+string(filepath.Separator)) {
			t.Fatalf("spill file should not be under workspace: %q", details.FullOutputPath)
		}
		if !strings.Contains(result.Text(), "output truncated") {
			t.Fatalf("text = %q", result.Text())
		}
	})

	t.Run("bash spill dir avoids workspace temp directories", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix temp fallback expectation")
		}
		dir := t.TempDir()
		workspaceTmp := filepath.Join(dir, "tmp")
		if err := os.MkdirAll(workspaceTmp, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		t.Setenv("TMPDIR", workspaceTmp)

		spillDir := defaultBashSpillDir(dir)
		if strings.HasPrefix(spillDir, dir+string(filepath.Separator)) {
			t.Fatalf("spill dir should not be under workspace: %q", spillDir)
		}
	})

	t.Run("bash cancellation interrupts command", func(t *testing.T) {
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := NewBashTool(dir).Run(ctx, Call{
			ID:   "call-1",
			Name: "bash",
			Args: map[string]any{"command": "sleep 5"},
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
		details, ok := result.Details.(BashResultDetails)
		if !ok || !details.Cancelled {
			t.Fatalf("details = %#v", result.Details)
		}
	})
}
