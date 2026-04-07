package prompt

import (
	_ "embed"
	"strings"
)

var (
	//go:embed reference/pi/default_system_prompt_template.md
	piDefaultSystemPromptTemplate string
)

func buildPiBasePrompt(tools []ToolPrompt) string {
	toolNames := make([]string, 0, len(tools))
	visibleTools := make([]string, 0, len(tools))
	guidelines := []string{}
	seenGuidelines := map[string]struct{}{}
	addGuideline := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if _, ok := seenGuidelines[text]; ok {
			return
		}
		seenGuidelines[text] = struct{}{}
		guidelines = append(guidelines, text)
	}

	for _, item := range tools {
		toolNames = append(toolNames, item.Name)
		if strings.TrimSpace(item.Snippet) != "" {
			visibleTools = append(visibleTools, "- "+item.Name+": "+strings.TrimSpace(item.Snippet))
		}
		for _, line := range item.Guidelines {
			addGuideline(line)
		}
	}

	has := func(name string) bool {
		for _, item := range toolNames {
			if item == name {
				return true
			}
		}
		return false
	}

	hasBash := has("bash")
	hasGrep := has("grep")
	hasFind := has("find")
	hasLs := has("ls")
	if hasBash && !hasGrep && !hasFind && !hasLs {
		addGuideline("Use bash for file operations like ls, rg, find")
	} else if hasBash && (hasGrep || hasFind || hasLs) {
		addGuideline("Prefer grep/find/ls tools over bash for file exploration (faster, respects .gitignore)")
	}
	addGuideline("Be concise in your responses")
	addGuideline("Show file paths clearly when working with files")

	toolsList := "(none)"
	if len(visibleTools) > 0 {
		toolsList = strings.Join(visibleTools, "\n")
	}
	guidelinesList := strings.Join(mapSlice(guidelines, func(item string) string {
		return "- " + item
	}), "\n")

	text := strings.NewReplacer(
		"${toolsList}", toolsList,
		"${guidelines}", guidelinesList,
		"${readmePath}", "README.md",
		"${docsPath}", "docs/",
		"${examplesPath}", "examples/",
	).Replace(strings.TrimSpace(piDefaultSystemPromptTemplate))

	return strings.NewReplacer(
		"inside pi, a coding agent harness", "inside fritz, a coding agent harness",
		"Pi documentation", "Fritz documentation",
		"pi itself", "fritz itself",
		"pi packages", "fritz packages",
		"working on pi topics", "working on fritz topics",
		"read pi .md files completely", "read fritz .md files completely",
	).Replace(text)
}

func mapSlice[T any, U any](items []T, fn func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, fn(item))
	}
	return out
}
