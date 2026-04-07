package prompt

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fritz/internal/memory"
	"fritz/internal/skill"
	"fritz/internal/tool"
)

type ContextFile struct {
	Path    string
	Content string
}

type Profile string

const (
	ProfileCoding  Profile = "coding"
	ProfileGateway Profile = "gateway"
)

type Resources struct {
	ContextFiles       []ContextFile
	MemoryFiles        []ContextFile
	HeartbeatFiles     []ContextFile
	SystemPrompt       string
	AppendSystemPrompt []string
	SkillRoots         []string
}

type DiscoverOptions struct {
	Cwd     string
	HomeDir string
}

type ToolPrompt struct {
	Name       string
	Snippet    string
	Guidelines []string
}

type BuildOptions struct {
	Profile            Profile
	Cwd                string
	Now                time.Time
	Base               string
	AppendSystemPrompt []string
	ContextFiles       []ContextFile
	MemoryFiles        []ContextFile
	HeartbeatFiles     []ContextFile
	Skills             []skill.Skill
	Tools              []ToolPrompt
}

func Discover(options DiscoverOptions) (Resources, error) {
	return discover(options, true, true)
}

func DiscoverForProfile(options DiscoverOptions, profile Profile) (Resources, error) {
	return discover(options, profile == ProfileGateway, profile == ProfileGateway)
}

func discover(options DiscoverOptions, includeMemory bool, includeHeartbeat bool) (Resources, error) {
	cwd := filepath.Clean(options.Cwd)
	home := filepath.Clean(options.HomeDir)
	var resources Resources

	if home != "." && home != "" {
		if file, ok := loadContextFile(filepath.Join(home, ".fritz")); ok {
			resources.ContextFiles = append(resources.ContextFiles, file)
		}
	}
	if includeMemory {
		memoryDocs, err := memory.Load(cwd)
		if err != nil {
			return Resources{}, err
		}
		for _, doc := range memoryDocs {
			resources.MemoryFiles = append(resources.MemoryFiles, ContextFile{
				Path:    doc.Path,
				Content: doc.Content,
			})
		}
	}
	var ancestors []ContextFile
	for _, dir := range walkAncestors(cwd) {
		if file, ok := loadContextFile(dir); ok {
			ancestors = append(ancestors, file)
		}
	}
	resources.ContextFiles = append(resources.ContextFiles, ancestors...)

	projectConfigDir := filepath.Join(cwd, ".fritz")
	if includeHeartbeat {
		if data, err := os.ReadFile(filepath.Join(cwd, "HEARTBEAT.md")); err == nil {
			resources.HeartbeatFiles = append(resources.HeartbeatFiles, ContextFile{
				Path:    filepath.Join(cwd, "HEARTBEAT.md"),
				Content: string(data),
			})
		}
	}
	if data, err := os.ReadFile(filepath.Join(projectConfigDir, "SYSTEM.md")); err == nil {
		resources.SystemPrompt = string(data)
	} else if home != "." && home != "" {
		if data, err := os.ReadFile(filepath.Join(home, ".fritz", "SYSTEM.md")); err == nil {
			resources.SystemPrompt = string(data)
		}
	}
	if data, err := os.ReadFile(filepath.Join(projectConfigDir, "APPEND_SYSTEM.md")); err == nil {
		resources.AppendSystemPrompt = append(resources.AppendSystemPrompt, string(data))
	} else if home != "." && home != "" {
		if data, err := os.ReadFile(filepath.Join(home, ".fritz", "APPEND_SYSTEM.md")); err == nil {
			resources.AppendSystemPrompt = append(resources.AppendSystemPrompt, string(data))
		}
	}

	var roots []string
	if home != "." && home != "" {
		roots = append(roots,
			filepath.Join(home, ".fritz", "skills"),
			filepath.Join(home, ".agents", "skills"),
		)
	}
	for _, dir := range walkAncestors(cwd) {
		roots = append(roots, filepath.Join(dir, ".agents", "skills"))
	}
	roots = append(roots, filepath.Join(cwd, ".fritz", "skills"))
	resources.SkillRoots = dedupeExistingDirs(roots)
	return resources, nil
}

func BuildSystemPrompt(options BuildOptions) string {
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	profile := options.Profile
	if profile == "" {
		profile = ProfileCoding
	}
	var builder strings.Builder
	base := strings.TrimSpace(options.Base)
	if base == "" {
		builder.WriteString(buildPiBasePrompt(options.Tools))
	} else {
		builder.WriteString(base)
	}

	for _, extra := range options.AppendSystemPrompt {
		extra = strings.TrimSpace(extra)
		if extra == "" {
			continue
		}
		builder.WriteString("\n\n")
		builder.WriteString(extra)
	}

	if profile == ProfileGateway {
		overlay := strings.TrimSpace(gatewayOverlayPromptBlock())
		if overlay != "" {
			builder.WriteString("\n\n")
			builder.WriteString(overlay)
		}
	}

	if len(options.ContextFiles) > 0 {
		builder.WriteString("\n\n# Project Context\n\n")
		builder.WriteString("Project-specific instructions and guidelines:\n\n")
		for _, file := range options.ContextFiles {
			builder.WriteString("## ")
			builder.WriteString(file.Path)
			builder.WriteString("\n\n")
			builder.WriteString(file.Content)
			builder.WriteString("\n\n")
		}
	}

	if profile == ProfileGateway && len(options.MemoryFiles) > 0 {
		builder.WriteString("\n\n# Durable Memory\n\n")
		builder.WriteString("Long-lived facts and notes persisted outside chat transcripts:\n\n")
		for _, file := range options.MemoryFiles {
			builder.WriteString("## ")
			builder.WriteString(file.Path)
			builder.WriteString("\n\n")
			builder.WriteString(file.Content)
			builder.WriteString("\n\n")
		}
	}

	if profile == ProfileGateway && len(options.HeartbeatFiles) > 0 {
		builder.WriteString("\n\n# Heartbeat Context\n\n")
		builder.WriteString("Heartbeat instructions and short checklists available to the agent:\n\n")
		for _, file := range options.HeartbeatFiles {
			builder.WriteString("## ")
			builder.WriteString(file.Path)
			builder.WriteString("\n\n")
			builder.WriteString(file.Content)
			builder.WriteString("\n\n")
		}
	}

	if text := skill.FormatForPrompt(options.Skills); text != "" {
		builder.WriteString(text)
	}

	builder.WriteString("\nCurrent date: ")
	builder.WriteString(now.UTC().Format("2006-01-02"))
	builder.WriteString("\nCurrent working directory: ")
	builder.WriteString(filepath.ToSlash(options.Cwd))
	return builder.String()
}

func ToolPromptsFromDefinitions(defs []tool.Definition) []ToolPrompt {
	out := make([]ToolPrompt, 0, len(defs))
	for _, def := range defs {
		snippet := strings.TrimSpace(def.PromptSnippet)
		if snippet == "" {
			snippet = strings.TrimSpace(def.Description)
		}
		out = append(out, ToolPrompt{
			Name:       def.Name,
			Snippet:    snippet,
			Guidelines: append([]string(nil), def.PromptGuidelines...),
		})
	}
	return out
}

func loadContextFile(dir string) (ContextFile, bool) {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			return ContextFile{Path: path, Content: string(data)}, true
		}
	}
	return ContextFile{}, false
}

func walkAncestors(start string) []string {
	current := filepath.Clean(start)
	var dirs []string
	for {
		dirs = append(dirs, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	slices.Reverse(dirs)
	return dirs
}

func dedupeExistingDirs(paths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
