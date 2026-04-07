package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkills(t *testing.T) {
	root := t.TempDir()
	validDir := filepath.Join(root, "task-pack-create")
	if err := os.MkdirAll(validDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte(`---
name: task-pack-create
description: Create task packs
---

# Skill
Use it.`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	disabledDir := filepath.Join(root, "secret-skill")
	if err := os.MkdirAll(disabledDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(disabledDir, "SKILL.md"), []byte(`---
description: Hidden
disable-model-invocation: true
---

hidden`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	badDir := filepath.Join(root, "Bad Name")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte(`---
name: Bad Name
---

missing desc`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result := Load(LoadOptions{Paths: []string{root}})
	if len(result.Skills) != 2 {
		t.Fatalf("Skills = %#v", result.Skills)
	}
	if result.Skills[0].Name != "secret-skill" && result.Skills[1].Name != "secret-skill" {
		t.Fatalf("Skills = %#v", result.Skills)
	}
	if result.Skills[0].Name != "task-pack-create" && result.Skills[1].Name != "task-pack-create" {
		t.Fatalf("Skills = %#v", result.Skills)
	}
	var hidden Skill
	for _, item := range result.Skills {
		if item.Name == "secret-skill" {
			hidden = item
		}
	}
	if !hidden.DisableModelInvocation {
		t.Fatalf("hidden skill = %#v", hidden)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected diagnostics")
	}
}

func TestFormatForPrompt(t *testing.T) {
	text := FormatForPrompt([]Skill{
		{
			Name:        "visible",
			Description: "Visible desc",
			FilePath:    "/tmp/visible/SKILL.md",
		},
		{
			Name:                   "hidden",
			Description:            "Hidden desc",
			FilePath:               "/tmp/hidden/SKILL.md",
			DisableModelInvocation: true,
		},
	})
	if strings.Contains(text, "hidden") {
		t.Fatalf("FormatForPrompt() = %q", text)
	}
	if !strings.Contains(text, "<available_skills>") || !strings.Contains(text, "visible") {
		t.Fatalf("FormatForPrompt() = %q", text)
	}
}

func TestExpandCommand(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "task-pack-create")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(`---
description: Create task packs
---

Use this skill.`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	skills := []Skill{{
		Name:        "task-pack-create",
		Description: "Create task packs",
		FilePath:    path,
		BaseDir:     skillDir,
	}}

	expanded, ok, err := ExpandCommand("/skill:task-pack-create build a pack", skills)
	if err != nil {
		t.Fatalf("ExpandCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("expected expansion")
	}
	if !strings.Contains(expanded, `<skill name="task-pack-create" location="`) {
		t.Fatalf("expanded = %q", expanded)
	}
	if !strings.Contains(expanded, "References are relative to "+skillDir+".") {
		t.Fatalf("expanded = %q", expanded)
	}
	if !strings.Contains(expanded, "build a pack") {
		t.Fatalf("expanded = %q", expanded)
	}
}

func TestExpandCommandUnknownSkill(t *testing.T) {
	_, _, err := ExpandCommand("/skill:nope", nil)
	if err == nil || err.Error() != `unknown skill "nope"` {
		t.Fatalf("ExpandCommand() error = %v", err)
	}
}
