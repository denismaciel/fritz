package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestUnknownCommandPrintsPlainError(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/fritz", "nope")
	cmd.Dir = "../.."

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected command to fail")
	}

	text := string(output)
	if !strings.Contains(text, `unknown command "nope"`) {
		t.Fatalf("expected unknown command output, got %q", text)
	}
	if strings.Contains(text, "20") || strings.Contains(text, "INFO") {
		t.Fatalf("expected plain error output, got %q", text)
	}
}
