package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fritz/internal/skill"
	"fritz/internal/tool"
)

func TestDiscoverOrdersContextFilesAndPromptFiles(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo", "nested")
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".fritz"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".fritz", "AGENTS.md"), []byte("global"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(cwd), "AGENTS.md"), []byte("parent"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "CLAUDE.md"), []byte("cwd"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfgDir := filepath.Join(cwd, ".fritz")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "MEMORY.md"), []byte("memory-main"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, "memory"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "memory", "alpha.md"), []byte("alpha"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "SYSTEM.md"), []byte("system file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "HEARTBEAT.md"), []byte("check reminders"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "APPEND_SYSTEM.md"), []byte("append file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "fritz", "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".agents", "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	resources, err := Discover(DiscoverOptions{Cwd: cwd, HomeDir: home})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(resources.ContextFiles) != 3 {
		t.Fatalf("ContextFiles = %#v", resources.ContextFiles)
	}
	if resources.ContextFiles[0].Content != "global" || resources.ContextFiles[1].Content != "parent" || resources.ContextFiles[2].Content != "cwd" {
		t.Fatalf("ContextFiles = %#v", resources.ContextFiles)
	}
	if resources.SystemPrompt != "system file" {
		t.Fatalf("SystemPrompt = %q", resources.SystemPrompt)
	}
	if len(resources.AppendSystemPrompt) != 1 || resources.AppendSystemPrompt[0] != "append file" {
		t.Fatalf("AppendSystemPrompt = %#v", resources.AppendSystemPrompt)
	}
	if len(resources.MemoryFiles) != 2 || resources.MemoryFiles[0].Content != "memory-main" || resources.MemoryFiles[1].Content != "alpha" {
		t.Fatalf("MemoryFiles = %#v", resources.MemoryFiles)
	}
	if len(resources.HeartbeatFiles) != 1 || resources.HeartbeatFiles[0].Content != "check reminders" {
		t.Fatalf("HeartbeatFiles = %#v", resources.HeartbeatFiles)
	}
	if len(resources.SkillRoots) != 2 {
		t.Fatalf("SkillRoots = %#v", resources.SkillRoots)
	}
	if resources.SkillRoots[0] != filepath.Join(home, ".config", "fritz", "skills") {
		t.Fatalf("SkillRoots = %#v", resources.SkillRoots)
	}
	if resources.SkillRoots[1] != filepath.Join(cwd, ".agents", "skills") {
		t.Fatalf("SkillRoots = %#v", resources.SkillRoots)
	}

	codingResources, err := DiscoverForProfile(DiscoverOptions{Cwd: cwd, HomeDir: home}, ProfileCoding)
	if err != nil {
		t.Fatalf("DiscoverForProfile() error = %v", err)
	}
	if len(codingResources.MemoryFiles) != 0 {
		t.Fatalf("coding MemoryFiles = %#v", codingResources.MemoryFiles)
	}
	if len(codingResources.HeartbeatFiles) != 0 {
		t.Fatalf("coding HeartbeatFiles = %#v", codingResources.HeartbeatFiles)
	}
}

func TestDiscoverUsesXDGGlobalSkillRoot(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(t.TempDir(), "xdg")
	cwd := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "fritz", "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "fritz", "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	resources, err := Discover(DiscoverOptions{Cwd: cwd, HomeDir: home})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(resources.SkillRoots) != 1 {
		t.Fatalf("SkillRoots = %#v", resources.SkillRoots)
	}
	if resources.SkillRoots[0] != filepath.Join(xdg, "fritz", "skills") {
		t.Fatalf("SkillRoots = %#v", resources.SkillRoots)
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	promptText := BuildSystemPrompt(BuildOptions{
		Profile: ProfileGateway,
		Cwd:     "/work/repo",
		Now:     time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Base:    "",
		ContextFiles: []ContextFile{
			{Path: "/work/AGENTS.md", Content: "be terse"},
		},
		MemoryFiles: []ContextFile{
			{Path: "/work/MEMORY.md", Content: "remember this"},
		},
		HeartbeatFiles: []ContextFile{
			{Path: "/work/HEARTBEAT.md", Content: "- check once"},
		},
		AppendSystemPrompt: []string{"extra rule"},
		Skills: []skill.Skill{
			{
				Name:        "task-pack-create",
				Description: "Create task packs",
				FilePath:    "/skills/task-pack-create/SKILL.md",
				BaseDir:     "/skills/task-pack-create",
			},
		},
		Tools: []ToolPrompt{
			{
				Name:       "read",
				Snippet:    "Read file contents",
				Guidelines: []string{"Use read before editing"},
			},
			{
				Name:       "bash",
				Snippet:    "Execute shell commands",
				Guidelines: []string{"Prefer rg over grep"},
			},
		},
	})

	assertContains(t, promptText, "Available tools:")
	assertContains(t, promptText, "- read: Read file contents")
	assertContains(t, promptText, "Use read before editing")
	assertContains(t, promptText, "# Project Context")
	assertContains(t, promptText, "be terse")
	assertContains(t, promptText, "# Durable Memory")
	assertContains(t, promptText, "remember this")
	assertContains(t, promptText, "# Heartbeat Context")
	assertContains(t, promptText, "HEARTBEAT_OK")
	assertContains(t, promptText, "WRITE IT TO A FILE")
	assertContains(t, promptText, "<available_skills>")
	assertContains(t, promptText, "task-pack-create")
	assertContains(t, promptText, "Current date: 2026-04-03")
	assertContains(t, promptText, "Current working directory: /work/repo")
	assertContains(t, promptText, "extra rule")
}

func TestBuildSystemPromptCodingProfileUsesFritzBaseAndSkipsGatewaySections(t *testing.T) {
	text := BuildSystemPrompt(BuildOptions{
		Profile: ProfileCoding,
		Cwd:     "/repo",
		Now:     time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		ContextFiles: []ContextFile{
			{Path: "/repo/AGENTS.md", Content: "be terse"},
		},
		MemoryFiles: []ContextFile{
			{Path: "/repo/MEMORY.md", Content: "remember this"},
		},
		HeartbeatFiles: []ContextFile{
			{Path: "/repo/HEARTBEAT.md", Content: "check it"},
		},
		Tools: []ToolPrompt{
			{Name: "read", Snippet: "Read file contents"},
			{Name: "bash", Snippet: "Execute shell commands"},
			{Name: "edit", Snippet: "Edit files with exact text replacement"},
			{Name: "write", Snippet: "Create or overwrite files"},
		},
	})

	assertContains(t, text, "You are an expert coding assistant operating inside fritz, a coding agent harness.")
	assertContains(t, text, "Fritz documentation")
	assertContains(t, text, "# Project Context")
	assertContains(t, text, "be terse")
	if strings.Contains(text, "# Durable Memory") || strings.Contains(text, "# Heartbeat Context") {
		t.Fatalf("coding prompt leaked gateway sections: %q", text)
	}
}

func TestBuildSystemPromptUsesCustomBase(t *testing.T) {
	text := BuildSystemPrompt(BuildOptions{
		Profile:            ProfileCoding,
		Cwd:                "/repo",
		Now:                time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Base:               "custom base",
		AppendSystemPrompt: []string{"tail"},
		Tools: []ToolPrompt{
			{Name: "read", Snippet: "Read file contents"},
		},
	})

	assertContains(t, text, "custom base")
	assertContains(t, text, "tail")
	assertContains(t, text, "Current working directory: /repo")
}

func TestToolPromptsFromDefinitions(t *testing.T) {
	defs := []tool.Definition{
		{Name: "read", PromptSnippet: "Read file contents", PromptGuidelines: []string{"Use read first"}},
		{Name: "bash", PromptSnippet: "Execute commands", PromptGuidelines: []string{"Prefer rg"}},
	}
	got := ToolPromptsFromDefinitions(defs)
	if len(got) != 2 {
		t.Fatalf("ToolPromptsFromDefinitions() = %#v", got)
	}
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q in %q", needle, haystack)
	}
}
