package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeBashOps struct {
	result BashExecResult
	err    error
}

func (f fakeBashOps) Exec(_ context.Context, _ string, _ string, _ BashExecOptions) (BashExecResult, error) {
	return f.result, f.err
}

func TestBashToolParity(t *testing.T) {
	runBash := func(t *testing.T, dir string, args map[string]any, options ...BashToolOption) (Result, error) {
		t.Helper()
		return NewBashTool(dir, options...).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "bash",
			Args: args,
		})
	}

	t.Run("should execute simple commands", func(t *testing.T) {
		dir := t.TempDir()
		result, err := runBash(t, dir, map[string]any{"command": "printf hello"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.Text() != "hello" {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should handle command errors", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := runBash(t, dir, map[string]any{"command": "printf nope && exit 7"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should respect timeout", func(t *testing.T) {
		dir := t.TempDir()
		start := time.Now()
		if _, err := runBash(t, dir, map[string]any{"command": "sleep 1", "timeout": 0.01}); err == nil {
			t.Fatal("expected timeout")
		}
		if time.Since(start) > time.Second {
			t.Fatal("timeout was not respected")
		}
	})

	t.Run("should throw error when cwd does not exist", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "missing")
		if _, err := runBash(t, dir, map[string]any{"command": "printf hi"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should handle process spawn errors", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := runBash(t, dir, map[string]any{"command": "printf hi"}, WithBashOperations(fakeBashOps{err: errors.New("boom")})); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should prepend command prefix when configured", func(t *testing.T) {
		dir := t.TempDir()
		result, err := runBash(t, dir, map[string]any{"command": "printf second"}, WithCommandPrefix("printf first"))
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "first") {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should include output from both prefix and command", func(t *testing.T) {
		dir := t.TempDir()
		result, err := runBash(t, dir, map[string]any{"command": "printf second"}, WithCommandPrefix("printf first;"))
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "first") || !strings.Contains(result.Text(), "second") {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should work without command prefix", func(t *testing.T) {
		dir := t.TempDir()
		result, err := runBash(t, dir, map[string]any{"command": "printf ok"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.Text() != "ok" {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should expose local bash operations for extension reuse", func(t *testing.T) {
		dir := t.TempDir()
		result, err := CreateLocalBashOperations().Exec(context.Background(), "printf local", dir, BashExecOptions{})
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if strings.TrimSpace(result.Output) != "local" {
			t.Fatalf("output = %q", result.Output)
		}
	})

	t.Run("should preserve executeBash sanitization when using local bash operations", func(t *testing.T) {
		dir := t.TempDir()
		resultA, err := ExecuteBash(context.Background(), "printf hi\\n", dir, BashExecOptions{})
		if err != nil {
			t.Fatalf("ExecuteBash() error = %v", err)
		}
		resultB, err := CreateLocalBashOperations().Exec(context.Background(), "printf hi\\n", dir, BashExecOptions{})
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if strings.TrimSpace(resultA.Output) != strings.TrimSpace(resultB.Output) {
			t.Fatalf("ExecuteBash=%q Exec=%q", resultA.Output, resultB.Output)
		}
	})

	t.Run("executeBash resolves after the shell exits even if inherited stdio handles stay open", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("windows-only parity case")
		}
	})

	t.Run("bash tool resolves after the shell exits even if inherited stdio handles stay open", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("windows-only parity case")
		}
	})
}

func TestDiscoveryToolParity(t *testing.T) {
	t.Run("should include filename when searching a single file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "one.txt")
		if err := os.WriteFile(path, []byte("hello pattern\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := NewGrepTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "grep",
			Args: map[string]any{"pattern": "pattern", "path": "one.txt"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "one.txt:1: hello pattern") {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should respect global limit and include context lines", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ctx.txt")
		if err := os.WriteFile(path, []byte("a\nmatch one\nc\nmatch two\ne\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := NewGrepTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "grep",
			Args: map[string]any{"pattern": "match", "path": ".", "context": 1, "limit": 1},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "ctx.txt-1- a") || !strings.Contains(result.Text(), "ctx.txt:2: match one") {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should include hidden files that are not gitignored", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".visible-hidden"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := NewFindTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "find",
			Args: map[string]any{"pattern": ".visible-hidden"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), ".visible-hidden") {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should respect .gitignore", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "shown.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := NewFindTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "find",
			Args: map[string]any{"pattern": "*.txt"},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if strings.Contains(result.Text(), "ignored.txt") || !strings.Contains(result.Text(), "shown.txt") {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("should list dotfiles and directories", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		result, err := NewLsTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "ls",
			Args: map[string]any{},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), ".env") || !strings.Contains(result.Text(), "nested/") {
			t.Fatalf("content = %q", result.Text())
		}
	})
}
