package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Skill struct {
	Name                   string
	Description            string
	FilePath               string
	BaseDir                string
	DisableModelInvocation bool
}

type Diagnostic struct {
	Path    string
	Message string
}

type LoadOptions struct {
	Paths []string
}

type LoadResult struct {
	Skills      []Skill
	Diagnostics []Diagnostic
}

func Load(options LoadOptions) LoadResult {
	seenPaths := map[string]struct{}{}
	byName := map[string]Skill{}
	var diagnostics []Diagnostic

	for _, root := range options.Paths {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: root, Message: err.Error()})
			continue
		}
		if info.IsDir() {
			skills, diags := loadFromDir(root, true)
			diagnostics = append(diagnostics, diags...)
			for _, item := range skills {
				if _, ok := seenPaths[item.FilePath]; ok {
					continue
				}
				if _, exists := byName[item.Name]; exists {
					diagnostics = append(diagnostics, Diagnostic{
						Path:    item.FilePath,
						Message: fmt.Sprintf("skill name collision: %s", item.Name),
					})
					continue
				}
				seenPaths[item.FilePath] = struct{}{}
				byName[item.Name] = item
			}
			continue
		}
		if !strings.HasSuffix(strings.ToLower(root), ".md") {
			diagnostics = append(diagnostics, Diagnostic{Path: root, Message: "skill path is not a markdown file"})
			continue
		}
		item, diags := loadSkillFile(root)
		diagnostics = append(diagnostics, diags...)
		if item == nil {
			continue
		}
		if _, ok := byName[item.Name]; ok {
			diagnostics = append(diagnostics, Diagnostic{Path: item.FilePath, Message: fmt.Sprintf("skill name collision: %s", item.Name)})
			continue
		}
		byName[item.Name] = *item
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	skills := make([]Skill, 0, len(names))
	for _, name := range names {
		skills = append(skills, byName[name])
	}

	return LoadResult{Skills: skills, Diagnostics: diagnostics}
}

func FormatForPrompt(skills []Skill) string {
	var visible []Skill
	for _, item := range skills {
		if !item.DisableModelInvocation {
			visible = append(visible, item)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines,
		"",
		"",
		"The following skills provide specialized instructions for specific tasks.",
		"Use the read tool to load a skill's file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory and use that absolute path in tool commands.",
		"",
		"<available_skills>",
	)
	for _, item := range visible {
		lines = append(lines,
			"  <skill>",
			fmt.Sprintf("    <name>%s</name>", escapeXML(item.Name)),
			fmt.Sprintf("    <description>%s</description>", escapeXML(item.Description)),
			fmt.Sprintf("    <location>%s</location>", escapeXML(item.FilePath)),
			"  </skill>",
		)
	}
	lines = append(lines, "</available_skills>")
	return strings.Join(lines, "\n")
}

func ExpandCommand(text string, skills []Skill) (string, bool, error) {
	if !strings.HasPrefix(text, "/skill:") {
		return text, false, nil
	}
	space := strings.Index(text, " ")
	name := text[len("/skill:"):]
	args := ""
	if space >= 0 {
		name = text[len("/skill:"):space]
		args = strings.TrimSpace(text[space+1:])
	}
	var selected *Skill
	for _, item := range skills {
		if item.Name == name {
			copy := item
			selected = &copy
			break
		}
	}
	if selected == nil {
		return "", false, fmt.Errorf(`unknown skill "%s"`, name)
	}
	body, err := readSkillBody(selected.FilePath)
	if err != nil {
		return "", false, err
	}
	block := fmt.Sprintf(
		"<skill name=%q location=%q>\nReferences are relative to %s.\n\n%s\n</skill>",
		selected.Name,
		selected.FilePath,
		selected.BaseDir,
		strings.TrimSpace(body),
	)
	if args != "" {
		block += "\n\n" + args
	}
	return block, true, nil
}

func loadFromDir(root string, includeRootFiles bool) ([]Skill, []Diagnostic) {
	var skills []Skill
	var diagnostics []Diagnostic

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, []Diagnostic{{Path: root, Message: err.Error()}}
	}
	for _, entry := range entries {
		if entry.Name() == "node_modules" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		full := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if _, err := os.Stat(filepath.Join(full, "SKILL.md")); err == nil {
				item, diags := loadSkillFile(filepath.Join(full, "SKILL.md"))
				diagnostics = append(diagnostics, diags...)
				if item != nil {
					skills = append(skills, *item)
				}
				continue
			}
			sub, diags := loadFromDir(full, false)
			diagnostics = append(diagnostics, diags...)
			skills = append(skills, sub...)
			continue
		}
		if includeRootFiles && strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			item, diags := loadSkillFile(full)
			diagnostics = append(diagnostics, diags...)
			if item != nil {
				skills = append(skills, *item)
			}
		}
	}
	return skills, diagnostics
}

func loadSkillFile(path string) (*Skill, []Diagnostic) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []Diagnostic{{Path: path, Message: err.Error()}}
	}
	meta, body := parseFrontmatter(string(data))
	var diagnostics []Diagnostic
	description := strings.TrimSpace(meta["description"])
	if description == "" {
		diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "description is required"})
		return nil, diagnostics
	}
	baseDir := filepath.Dir(path)
	parent := filepath.Base(baseDir)
	name := strings.TrimSpace(meta["name"])
	if name == "" {
		name = parent
	}
	if !validSkillName(name) {
		diagnostics = append(diagnostics, Diagnostic{Path: path, Message: fmt.Sprintf("invalid skill name: %s", name)})
	}
	_ = body
	return &Skill{
		Name:                   name,
		Description:            description,
		FilePath:               path,
		BaseDir:                baseDir,
		DisableModelInvocation: strings.EqualFold(strings.TrimSpace(meta["disable-model-invocation"]), "true"),
	}, diagnostics
}

func readSkillBody(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	_, body := parseFrontmatter(string(data))
	body = strings.TrimSpace(body)
	if body == "" {
		return "", errors.New("skill body is empty")
	}
	return body, nil
}

func parseFrontmatter(content string) (map[string]string, string) {
	meta := map[string]string{}
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return meta, content
	}
	rest := content[4:]
	if strings.HasPrefix(content, "---\r\n") {
		rest = content[5:]
	}
	lines := strings.Split(rest, "\n")
	end := -1
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if line == "---" {
			end = i
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if end == -1 {
		return map[string]string{}, content
	}
	body := strings.Join(lines[end+1:], "\n")
	return meta, body
}

func validSkillName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
			if i == 0 || i == len(name)-1 {
				return false
			}
		default:
			return false
		}
	}
	return !strings.Contains(name, "--")
}

func escapeXML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
