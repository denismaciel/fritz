package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bradleyjkemp/cupaloy/v2"

	"fritz/internal/config"
	"fritz/internal/skill"
	"fritz/internal/tool"
)

func TestSystemPromptSnapshot(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(cwd, ".fritz", "memory"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".fritz", "skills", "memory-maintainer"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".fritz", "skills", "telegram-operator"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeTestFile(t, filepath.Join(cwd, "AGENTS.md"), "Project rule: be terse.")
	writeTestFile(t, filepath.Join(cwd, "MEMORY.md"), "User likes terse status updates.")
	writeTestFile(t, filepath.Join(cwd, "memory", "friends.md"), "Alice likes sushi.")
	writeTestFile(t, filepath.Join(cwd, "HEARTBEAT.md"), "- Check reminders.\n- Stay quiet if nothing new.")
	writeTestFile(t, filepath.Join(cwd, ".fritz", "skills", "memory-maintainer", "SKILL.md"), strings.TrimSpace(`
---
description: Keep durable memory files tidy and current.
---

# Memory Maintainer

Review memory files and update them carefully.
`))
	writeTestFile(t, filepath.Join(cwd, ".fritz", "skills", "telegram-operator", "SKILL.md"), strings.TrimSpace(`
---
description: Handle Telegram-facing replies and etiquette.
---

# Telegram Operator

Reply tersely and avoid spam.
`))

	runtime, err := LoadRuntime(cwd, config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
	}))
	if err != nil {
		t.Fatalf("LoadRuntime() error = %v", err)
	}

	registry := tool.NewRegistry()
	registry.Register(tool.NewReadTool(cwd, 128*1024))
	registry.Register(tool.NewWriteTool(cwd))
	registry.Register(tool.NewBashTool(cwd))
	registry.Register(tool.NewWebSearchTool("test-key", "https://generativelanguage.googleapis.com"))

	promptText := BuildSystemPrompt(BuildOptions{
		Profile:            runtime.Profile,
		Cwd:                cwd,
		Now:                time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Base:               runtime.Resources.SystemPrompt,
		AppendSystemPrompt: runtime.Resources.AppendSystemPrompt,
		ContextFiles:       runtime.Resources.ContextFiles,
		MemoryFiles:        runtime.Resources.MemoryFiles,
		HeartbeatFiles:     runtime.Resources.HeartbeatFiles,
		Skills:             runtime.Skills,
		Tools:              ToolPromptsFromDefinitions(registry.Definitions()),
	})

	promptText = normalizeSnapshotPrompt(promptText, cwd, runtime.Skills)

	snapshot := cupaloy.New(cupaloy.SnapshotSubdirectory("testdata/snapshots"))
	snapshot.SnapshotT(t, promptText)
}

func TestGatewaySystemPromptIncludesMemoryAndHeartbeat(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "repo")
	writeTestFile(t, filepath.Join(cwd, "AGENTS.md"), "Project rule: be terse.")
	writeTestFile(t, filepath.Join(cwd, "MEMORY.md"), "User likes terse status updates.")
	writeTestFile(t, filepath.Join(cwd, "HEARTBEAT.md"), "- Check reminders.")

	runtime, err := LoadRuntimeForProfile(cwd, config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
	}), ProfileGateway)
	if err != nil {
		t.Fatalf("LoadRuntimeForProfile() error = %v", err)
	}

	text := BuildSystemPrompt(BuildOptions{
		Profile:            runtime.Profile,
		Cwd:                cwd,
		Now:                time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Base:               runtime.Resources.SystemPrompt,
		AppendSystemPrompt: runtime.Resources.AppendSystemPrompt,
		ContextFiles:       runtime.Resources.ContextFiles,
		MemoryFiles:        runtime.Resources.MemoryFiles,
		HeartbeatFiles:     runtime.Resources.HeartbeatFiles,
		Skills:             runtime.Skills,
		Tools: []ToolPrompt{
			{Name: "read", Snippet: "Read file contents"},
		},
	})

	if !strings.Contains(text, "# Durable Memory") || !strings.Contains(text, "# Heartbeat Context") {
		t.Fatalf("gateway prompt missing memory/heartbeat sections: %q", text)
	}
}

func normalizeSnapshotPrompt(text string, cwd string, skills []skill.Skill) string {
	text = strings.ReplaceAll(text, filepath.ToSlash(cwd), "/work/repo")
	text = strings.ReplaceAll(text, cwd, "/work/repo")
	for _, item := range skills {
		text = strings.ReplaceAll(text, item.FilePath, strings.Replace(item.FilePath, cwd, "/work/repo", 1))
		text = strings.ReplaceAll(text, filepath.ToSlash(item.FilePath), strings.Replace(filepath.ToSlash(item.FilePath), filepath.ToSlash(cwd), "/work/repo", 1))
	}
	return text
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
