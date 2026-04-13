package tool

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteToolParity(t *testing.T) {
	runWrite := func(t *testing.T, dir string, args map[string]any) (Result, error) {
		t.Helper()
		return NewWriteTool(dir).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "write",
			Args: args,
		})
	}

	t.Run("should write file contents", func(t *testing.T) {
		dir := t.TempDir()
		result, err := runWrite(t, dir, map[string]any{"path": "write-test.txt", "content": "Test content"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "Successfully wrote") {
			t.Fatalf("content = %q", result.Text())
		}
		details, ok := result.Details.(WriteResultDetails)
		if !ok {
			t.Fatalf("Details type = %T", result.Details)
		}
		if !strings.Contains(details.Diff, "+Test content") {
			t.Fatalf("diff = %q", details.Diff)
		}
	})

	t.Run("should create parent directories", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := runWrite(t, dir, map[string]any{"path": "nested/dir/test.txt", "content": "Nested content"}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "nested/dir/test.txt")); err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
	})
}

func TestEditToolCoreParity(t *testing.T) {
	runEdit := func(t *testing.T, dir string, args map[string]any) (Result, error) {
		t.Helper()
		return NewEditTool(dir, DefaultReadMaxBytes).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "edit",
			Args: args,
		})
	}

	t.Run("should replace text in file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "edit-test.txt")
		if err := os.WriteFile(path, []byte("Hello, world!"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{
			"path": "edit-test.txt",
			"edits": []any{
				map[string]any{"oldText": "world", "newText": "gopher"},
			},
		}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "Hello, gopher!" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should fail if text not found", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "missing.txt")
		if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{"path": "missing.txt", "edits": []any{map[string]any{"oldText": "nope", "newText": "x"}}}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should fail if text appears multiple times", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "dupe.txt")
		if err := os.WriteFile(path, []byte("hello\nhello\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{"path": "dupe.txt", "edits": []any{map[string]any{"oldText": "hello", "newText": "x"}}}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should replace multiple disjoint regions in one call", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "multi.txt")
		if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{
			"path": "multi.txt",
			"edits": []any{
				map[string]any{"oldText": "alpha\n", "newText": "ALPHA\n"},
				map[string]any{"oldText": "gamma\n", "newText": "GAMMA\n"},
			},
		}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "ALPHA\nbeta\nGAMMA\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should collapse large unchanged gaps in multi-edit diffs", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "large-edit.txt")
		lines := make([]string, 1000)
		for i := range lines {
			lines[i] = "line " + strconv.Itoa(i)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runEdit(t, dir, map[string]any{
			"path": "large-edit.txt",
			"edits": []any{
				map[string]any{"oldText": "line 50\n", "newText": "line 50 changed\n"},
				map[string]any{"oldText": "line 950\n", "newText": "line 950 changed\n"},
			},
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		details, ok := result.Details.(EditResultDetails)
		if !ok || !strings.Contains(details.Diff, "unchanged lines") {
			t.Fatalf("details = %#v", result.Details)
		}
	})

	t.Run("should match edits against the original file, not incrementally", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "original.txt")
		if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{
			"path": "original.txt",
			"edits": []any{
				map[string]any{"oldText": "three\n", "newText": "THREE\n"},
				map[string]any{"oldText": "one\n", "newText": "ONE\n"},
			},
		}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "ONE\ntwo\nTHREE\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should fail when edits is empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{"path": "empty.txt", "edits": []any{}}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should fail when multi-edit regions overlap", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "overlap.txt")
		if err := os.WriteFile(path, []byte("abcde\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{
			"path": "overlap.txt",
			"edits": []any{
				map[string]any{"oldText": "abc", "newText": "ABC"},
				map[string]any{"oldText": "bcd", "newText": "BCD"},
			},
		}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should not partially apply edits when one edit fails", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "partial.txt")
		original := "alpha\nbeta\ngamma\n"
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runEdit(t, dir, map[string]any{
			"path": "partial.txt",
			"edits": []any{
				map[string]any{"oldText": "alpha\n", "newText": "ALPHA\n"},
				map[string]any{"oldText": "missing\n", "newText": "MISSING\n"},
			},
		}); err == nil {
			t.Fatal("expected error")
		}
		data, _ := os.ReadFile(path)
		if string(data) != original {
			t.Fatalf("file changed: %q", string(data))
		}
	})
}

func TestEditToolFuzzyParity(t *testing.T) {
	runEdit := func(t *testing.T, dir string, args map[string]any) error {
		t.Helper()
		_, err := NewEditTool(dir, DefaultReadMaxBytes).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "edit",
			Args: args,
		})
		return err
	}

	t.Run("should match text with trailing whitespace stripped", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "trailing.txt")
		if err := os.WriteFile(path, []byte("line one   \nline two  \nline three\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "trailing.txt", "edits": []any{map[string]any{"oldText": "line one\nline two\n", "newText": "replaced\n"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "replaced\nline three\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should match fullwidth punctuation in Chinese text", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "chinese.txt")
		if err := os.WriteFile(path, []byte("你好，世界\n你好（世界）\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "chinese.txt", "edits": []any{map[string]any{"oldText": "你好,世界\n你好(世界)\n", "newText": "你好，pi\n你好(pi)\n"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("should match compatibility-equivalent Unicode forms", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "compat.txt")
		if err := os.WriteFile(path, []byte("ＡＢＣ１２３\ncafe\u0301\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "compat.txt", "edits": []any{map[string]any{"oldText": "ABC123\ncafé\n", "newText": "XYZ789\ncoffee\n"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("should match smart single quotes to ASCII quotes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "quotes.txt")
		if err := os.WriteFile(path, []byte("console.log(\u2018hello\u2019);\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "quotes.txt", "edits": []any{map[string]any{"oldText": "console.log('hello');", "newText": "console.log('world');"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("should match smart double quotes to ASCII quotes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "double-quotes.txt")
		if err := os.WriteFile(path, []byte("const msg = \u201cHello World\u201d;\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "double-quotes.txt", "edits": []any{map[string]any{"oldText": `const msg = "Hello World";`, "newText": `const msg = "Goodbye";`}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("should match Unicode dashes to ASCII hyphen", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "dashes.txt")
		if err := os.WriteFile(path, []byte("range: 1\u20135\nbreak\u2014here\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "dashes.txt", "edits": []any{map[string]any{"oldText": "range: 1-5\nbreak-here", "newText": "range: 10-50\nbreak--here"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("should match non-breaking space to regular space", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nbsp.txt")
		if err := os.WriteFile(path, []byte("hello\u00a0world\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "nbsp.txt", "edits": []any{map[string]any{"oldText": "hello world", "newText": "hello universe"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("should prefer exact match over fuzzy match", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "exact.txt")
		original := "const x = 'exact';\nconst y = 'other';\n"
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "exact.txt", "edits": []any{map[string]any{"oldText": "const x = 'exact';", "newText": "const x = 'changed';"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "const x = 'changed';\nconst y = 'other';\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should still fail when text is not found even with fuzzy matching", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "no-match.txt")
		if err := os.WriteFile(path, []byte("completely different content\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "no-match.txt", "edits": []any{map[string]any{"oldText": "this does not exist", "newText": "replacement"}}}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should detect duplicates after fuzzy normalization", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "dups.txt")
		if err := os.WriteFile(path, []byte("hello world   \nhello world\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "dups.txt", "edits": []any{map[string]any{"oldText": "hello world", "newText": "replaced"}}}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should support fuzzy matching in multi-edit mode", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "multi-fuzzy.txt")
		if err := os.WriteFile(path, []byte("console.log(\u2018hello\u2019);\nhello\u00a0world\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{
			"path": "multi-fuzzy.txt",
			"edits": []any{
				map[string]any{"oldText": "console.log('hello');\n", "newText": "console.log('world');\n"},
				map[string]any{"oldText": "hello world\n", "newText": "hello universe\n"},
			},
		}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})
}

func TestEditToolCRLFParity(t *testing.T) {
	runEdit := func(t *testing.T, dir string, args map[string]any) error {
		t.Helper()
		_, err := NewEditTool(dir, DefaultReadMaxBytes).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "edit",
			Args: args,
		})
		return err
	}

	t.Run("should match LF oldText against CRLF file content", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "crlf-test.txt")
		if err := os.WriteFile(path, []byte("line one\r\nline two\r\nline three\r\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "crlf-test.txt", "edits": []any{map[string]any{"oldText": "line two\n", "newText": "replaced line\n"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("should preserve CRLF line endings after edit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "crlf-preserve.txt")
		if err := os.WriteFile(path, []byte("first\r\nsecond\r\nthird\r\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "crlf-preserve.txt", "edits": []any{map[string]any{"oldText": "second\n", "newText": "REPLACED\n"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "first\r\nREPLACED\r\nthird\r\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should preserve LF line endings for LF files", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lf-preserve.txt")
		if err := os.WriteFile(path, []byte("first\nsecond\nthird\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "lf-preserve.txt", "edits": []any{map[string]any{"oldText": "second\n", "newText": "REPLACED\n"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "first\nREPLACED\nthird\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should detect duplicates across CRLF/LF variants", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "mixed.txt")
		if err := os.WriteFile(path, []byte("hello\r\nworld\r\n---\r\nhello\nworld\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "mixed.txt", "edits": []any{map[string]any{"oldText": "hello\nworld\n", "newText": "replaced\n"}}}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should preserve UTF-8 BOM after edit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bom.txt")
		if err := os.WriteFile(path, []byte("\ufefffirst\r\nsecond\r\nthird\r\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{"path": "bom.txt", "edits": []any{map[string]any{"oldText": "second\n", "newText": "REPLACED\n"}}}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "\ufefffirst\r\nREPLACED\r\nthird\r\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("should preserve CRLF line endings and BOM in multi-edit mode", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bom-multi.txt")
		if err := os.WriteFile(path, []byte("\ufefffirst\r\nsecond\r\nthird\r\nfourth\r\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := runEdit(t, dir, map[string]any{
			"path": "bom-multi.txt",
			"edits": []any{
				map[string]any{"oldText": "second\n", "newText": "SECOND\n"},
				map[string]any{"oldText": "fourth\n", "newText": "FOURTH\n"},
			},
		}); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "\ufefffirst\r\nSECOND\r\nthird\r\nFOURTH\r\n" {
			t.Fatalf("file = %q", string(data))
		}
	})
}

func TestEditLegacyAndQueueParity(t *testing.T) {
	def := NewEditTool(".", DefaultReadMaxBytes).Definition()

	t.Run("keeps legacy fields out of the public schema", func(t *testing.T) {
		if _, ok := def.Parameters.Properties["oldText"]; ok {
			t.Fatal("oldText should not be public")
		}
		if _, ok := def.Parameters.Properties["newText"]; ok {
			t.Fatal("newText should not be public")
		}
	})

	t.Run("folds top-level oldText/newText into edits", func(t *testing.T) {
		prepared := def.PrepareArgs(map[string]any{"path": "file.txt", "oldText": "before", "newText": "after"}).(map[string]any)
		edits := prepared["edits"].([]map[string]any)
		if len(edits) != 1 || edits[0]["oldText"] != "before" || edits[0]["newText"] != "after" {
			t.Fatalf("prepared = %#v", prepared)
		}
	})

	t.Run("appends legacy replacement to existing edits", func(t *testing.T) {
		prepared := def.PrepareArgs(map[string]any{
			"path":    "file.txt",
			"edits":   []any{map[string]any{"oldText": "a", "newText": "b"}},
			"oldText": "c",
			"newText": "d",
		}).(map[string]any)
		edits := prepared["edits"].([]map[string]any)
		if len(edits) != 2 {
			t.Fatalf("prepared = %#v", prepared)
		}
	})

	t.Run("passes through valid input unchanged", func(t *testing.T) {
		input := map[string]any{
			"path":  "file.txt",
			"edits": []any{map[string]any{"oldText": "a", "newText": "b"}},
		}
		prepared := def.PrepareArgs(input)
		if !reflect.DeepEqual(prepared, input) {
			t.Fatalf("prepared = %#v input = %#v", prepared, input)
		}
	})

	t.Run("passes through non-object input unchanged", func(t *testing.T) {
		if def.PrepareArgs(nil) != nil {
			t.Fatal("expected nil passthrough")
		}
		if got := def.PrepareArgs("garbage"); got != "garbage" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("prepared args execute correctly", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "legacy.txt")
		if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		prepared := def.PrepareArgs(map[string]any{"path": "legacy.txt", "oldText": "before", "newText": "after"}).(map[string]any)
		result, err := NewEditTool(dir, DefaultReadMaxBytes).Run(context.Background(), Call{ID: "tool-1", Name: "edit", Args: prepared})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "Successfully replaced 1 block(s) in legacy.txt.") {
			t.Fatalf("content = %q", result.Text())
		}
	})

	t.Run("serializes operations for the same file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "same.txt")
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan struct{})

		go func() {
			_ = withFileMutationQueue(path, func() error {
				close(started)
				<-release
				return nil
			})
		}()
		<-started
		go func() {
			_ = withFileMutationQueue(path, func() error {
				close(done)
				return nil
			})
		}()
		select {
		case <-done:
			t.Fatal("second operation should block")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("second operation did not proceed")
		}
	})

	t.Run("allows different files to proceed in parallel", func(t *testing.T) {
		dir := t.TempDir()
		first := filepath.Join(dir, "a.txt")
		second := filepath.Join(dir, "b.txt")
		started := make(chan string, 2)
		release := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		for _, path := range []string{first, second} {
			path := path
			go func() {
				defer wg.Done()
				_ = withFileMutationQueue(path, func() error {
					started <- path
					<-release
					return nil
				})
			}()
		}
		a := <-started
		b := <-started
		if a == b {
			t.Fatal("expected parallel starts")
		}
		close(release)
		wg.Wait()
	})

	t.Run("uses the same queue for symlink aliases", func(t *testing.T) {
		dir := t.TempDir()
		realPath := filepath.Join(dir, "real.txt")
		linkPath := filepath.Join(dir, "link.txt")
		if err := os.WriteFile(realPath, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.Symlink(realPath, linkPath); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan struct{})
		go func() {
			_ = withFileMutationQueue(realPath, func() error {
				close(started)
				<-release
				return nil
			})
		}()
		<-started
		go func() {
			_ = withFileMutationQueue(linkPath, func() error {
				close(done)
				return nil
			})
		}()
		select {
		case <-done:
			t.Fatal("alias should share queue")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("alias queue did not unblock")
		}
	})

	t.Run("preserves both parallel edits on the same file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "parallel.txt")
		if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = NewEditTool(dir, DefaultReadMaxBytes).Run(context.Background(), Call{
				ID:   "1",
				Name: "edit",
				Args: map[string]any{"path": "parallel.txt", "edits": []any{map[string]any{"oldText": "alpha\n", "newText": "ALPHA\n"}}},
			})
		}()
		go func() {
			defer wg.Done()
			_, _ = NewEditTool(dir, DefaultReadMaxBytes).Run(context.Background(), Call{
				ID:   "2",
				Name: "edit",
				Args: map[string]any{"path": "parallel.txt", "edits": []any{map[string]any{"oldText": "beta\n", "newText": "BETA\n"}}},
			})
		}()
		wg.Wait()
		data, _ := os.ReadFile(path)
		if string(data) != "ALPHA\nBETA\n" {
			t.Fatalf("file = %q", string(data))
		}
	})

	t.Run("shares the queue between edit and write", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "shared.txt")
		if err := os.WriteFile(path, []byte("alpha\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan struct{})
		go func() {
			_ = withFileMutationQueue(path, func() error {
				close(started)
				<-release
				return nil
			})
		}()
		<-started
		go func() {
			_, _ = NewWriteTool(dir).Run(context.Background(), Call{
				ID:   "call-1",
				Name: "write",
				Args: map[string]any{"path": "shared.txt", "content": "beta\n"},
			})
			close(done)
		}()
		select {
		case <-done:
			t.Fatal("write should wait for queue")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("write did not proceed")
		}
	})
}
