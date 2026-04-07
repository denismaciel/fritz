package tool

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x60, 0x60, 0x60, 0xf8,
	0x0f, 0x00, 0x01, 0x04, 0x01, 0x00, 0x5f, 0xe5,
	0xc3, 0x4b, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestPathUtilsParity(t *testing.T) {
	t.Run("should expand ~ to home directory", func(t *testing.T) {
		got := ExpandPath("~")
		if got == "~" {
			t.Fatal("expected expanded home path")
		}
	})

	t.Run("should expand ~/path to home directory", func(t *testing.T) {
		got := ExpandPath("~/Documents/file.txt")
		if strings.Contains(got, "~/") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("should normalize Unicode spaces", func(t *testing.T) {
		got := ExpandPath("file\u00a0name.txt")
		if got != "file name.txt" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("should resolve absolute paths as-is", func(t *testing.T) {
		got := ResolveToCwd("/absolute/path/file.txt", "/some/cwd")
		if got != filepath.Clean("/absolute/path/file.txt") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("should resolve relative paths against cwd", func(t *testing.T) {
		got := ResolveToCwd("relative/file.txt", "/some/cwd")
		want := filepath.Clean("/some/cwd/relative/file.txt")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("should resolve existing file path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test-file.txt")
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		got := ResolveReadPath("test-file.txt", dir)
		if got != path {
			t.Fatalf("got %q want %q", got, path)
		}
	})

	t.Run("should handle NFC vs NFD Unicode normalization (macOS filenames with accents)", func(t *testing.T) {
		dir := t.TempDir()
		nfdName := "file\u0065\u0301.txt"
		nfcName := "file\u00e9.txt"
		path := filepath.Join(dir, nfdName)
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		got := ResolveReadPath(nfcName, dir)
		if got != path {
			t.Fatalf("got %q want %q", got, path)
		}
	})

	t.Run("should handle curly quotes vs straight quotes (macOS filenames)", func(t *testing.T) {
		dir := t.TempDir()
		curlyName := "Capture d\u2019cran.txt"
		straightName := "Capture d'cran.txt"
		path := filepath.Join(dir, curlyName)
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		got := ResolveReadPath(straightName, dir)
		if got != path {
			t.Fatalf("got %q want %q", got, path)
		}
	})

	t.Run("should handle combined NFC + curly quote (French macOS screenshots)", func(t *testing.T) {
		dir := t.TempDir()
		curly := "Capture d\u2019\u00e9cran.txt"
		straight := "Capture d'\u00e9cran.txt"
		path := filepath.Join(dir, curly)
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		got := ResolveReadPath(straight, dir)
		if got != path {
			t.Fatalf("got %q want %q", got, path)
		}
	})

	t.Run("should handle macOS screenshot AM/PM variant with narrow no-break space", func(t *testing.T) {
		dir := t.TempDir()
		macosName := "Screenshot 2024-01-01 at 10.00.00\u202fAM.png"
		userName := "Screenshot 2024-01-01 at 10.00.00 AM.png"
		path := filepath.Join(dir, macosName)
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		got := ResolveReadPath(userName, dir)
		if got != path {
			t.Fatalf("got %q want %q", got, path)
		}
	})
}

func TestReadToolParity(t *testing.T) {
	runRead := func(t *testing.T, dir string, args map[string]any) (Result, error) {
		t.Helper()
		return NewReadTool(dir, DefaultReadMaxBytes).Run(context.Background(), Call{
			ID:   "call-1",
			Name: "read",
			Args: args,
		})
	}

	t.Run("should read file contents that fit within limits", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := "Hello, world!\nLine 2\nLine 3"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "test.txt"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.Text() != content {
			t.Fatalf("content = %q", result.Text())
		}
		if result.Details != nil {
			t.Fatalf("expected no details, got %#v", result.Details)
		}
	})

	t.Run("should handle non-existent files", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := runRead(t, dir, map[string]any{"path": "missing.txt"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("should truncate files exceeding line limit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "large.txt")
		lines := make([]string, 2500)
		for i := range lines {
			lines[i] = "Line " + strconv.Itoa(i+1)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "large.txt"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "Line 2000") || strings.Contains(result.Text(), "Line 2001") {
			t.Fatalf("unexpected truncation output")
		}
		if !strings.Contains(result.Text(), "Use offset=2001") {
			t.Fatalf("missing continuation hint: %q", result.Text())
		}
	})

	t.Run("should truncate when byte limit exceeded", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "large-bytes.txt")
		lines := make([]string, 500)
		for i := range lines {
			lines[i] = "Line " + strconv.Itoa(i+1) + ": " + strings.Repeat("x", 200)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "large-bytes.txt"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "(50.0KB limit)") {
			t.Fatalf("missing byte-limit hint: %q", result.Text())
		}
	})

	t.Run("should handle offset parameter", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "offset-test.txt")
		lines := make([]string, 100)
		for i := range lines {
			lines[i] = "Line " + strconv.Itoa(i+1)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "offset-test.txt", "offset": 51})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if strings.Contains(result.Text(), "Line 50") || !strings.Contains(result.Text(), "Line 51") {
			t.Fatalf("unexpected output: %q", result.Text())
		}
	})

	t.Run("should handle limit parameter", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "limit-test.txt")
		lines := make([]string, 100)
		for i := range lines {
			lines[i] = "Line " + strconv.Itoa(i+1)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "limit-test.txt", "limit": 10})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "[90 more lines in file. Use offset=11 to continue.]") {
			t.Fatalf("unexpected output: %q", result.Text())
		}
	})

	t.Run("should handle offset + limit together", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "offset-limit-test.txt")
		lines := make([]string, 100)
		for i := range lines {
			lines[i] = "Line " + strconv.Itoa(i+1)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "offset-limit-test.txt", "offset": 41, "limit": 20})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "[40 more lines in file. Use offset=61 to continue.]") {
			t.Fatalf("unexpected output: %q", result.Text())
		}
	})

	t.Run("should show error when offset is beyond file length", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "short.txt")
		if err := os.WriteFile(path, []byte("Line 1\nLine 2\nLine 3"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := runRead(t, dir, map[string]any{"path": "short.txt", "offset": 100}); err == nil || !strings.Contains(err.Error(), "Offset 100 is beyond end of file") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("should include truncation details when truncated", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "large-file.txt")
		lines := make([]string, 2500)
		for i := range lines {
			lines[i] = "Line " + strconv.Itoa(i+1)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "large-file.txt"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		details, ok := result.Details.(*ReadResultDetails)
		if !ok || details == nil || details.Truncation == nil {
			t.Fatalf("missing truncation details: %#v", result.Details)
		}
	})

	t.Run("should detect image MIME type from file magic (not extension)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "image.txt")
		if err := os.WriteFile(path, tinyPNG, 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "image.txt"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "Read image file [image/png]") {
			t.Fatalf("unexpected content: %q", result.Text())
		}
		image, ok := result.FirstImage()
		if !ok || image.MIMEType != "image/png" {
			t.Fatalf("details = %#v", result)
		}
	})

	t.Run("should treat files with image extension but non-image content as text", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "not-an-image.png")
		if err := os.WriteFile(path, []byte("definitely not a png"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "not-an-image.png"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(result.Text(), "definitely not a png") {
			t.Fatalf("unexpected content: %q", result.Text())
		}
		if _, ok := result.FirstImage(); ok {
			t.Fatalf("unexpected image details: %#v", result.Details)
		}
	})

	t.Run("should always read images (filtering happens at convertToLlm layer)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "image.png")
		if err := os.WriteFile(path, tinyPNG, 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "image.png"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		image, ok := result.FirstImage()
		if !ok || image.Data == "" {
			t.Fatalf("missing image payload: %#v", result)
		}
	})

	t.Run("should read text files normally", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "plain.txt")
		if err := os.WriteFile(path, []byte("plain text"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		result, err := runRead(t, dir, map[string]any{"path": "plain.txt"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.Text() != "plain text" {
			t.Fatalf("content = %q", result.Text())
		}
	})
}
