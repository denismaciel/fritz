package expansion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandFiles(t *testing.T) {
	tmp := t.TempDir()
	readme := filepath.Join(tmp, "README.md")
	if err := os.WriteFile(readme, []byte("Hello README"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected string
		wantErr  bool
	}{
		{
			name:     "no expansion",
			text:     "hello world",
			expected: "hello world",
		},
		{
			name:     "expand README.md",
			text:     "Summarize @README.md",
			expected: "Summarize \n\nFile: README.md\n```\nHello README\n```\n",
		},
		{
			name:    "file not found",
			text:    "Summarize @missing.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandFiles(tt.text, tmp)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExpandFiles() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.expected {
				t.Fatalf("ExpandFiles() = %q, expected %q", got, tt.expected)
			}
		})
	}
}
