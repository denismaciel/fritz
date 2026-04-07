package prompt

import (
	_ "embed"
	"strings"
)

var (
	//go:embed reference/openclaw/heartbeat_prompt.txt
	openClawHeartbeatPrompt string

	//go:embed reference/openclaw/memory_sections.md
	openClawMemorySections string
)

func defaultHarnessPromptBlock() string {
	memoryText := stripHTMLComments(openClawMemorySections)
	heartbeatText := strings.TrimSpace(openClawHeartbeatPrompt)

	var builder strings.Builder
	builder.WriteString("Harness conventions:\n")
	builder.WriteString("- Durable memory guidance follows:\n\n")
	builder.WriteString(memoryText)
	builder.WriteString("\nHeartbeat contract:\n")
	builder.WriteString("- Some runs may be heartbeat runs rather than direct user messages.\n")
	builder.WriteString("- Default heartbeat prompt: ")
	builder.WriteString(heartbeatText)
	builder.WriteString("\n")
	builder.WriteString("- If heartbeat context exists, follow it strictly.\n")
	builder.WriteString("- If nothing is actionable during a heartbeat run, reply exactly HEARTBEAT_OK.\n")
	builder.WriteString("Secrets:\n")
	builder.WriteString("- Do not write API keys, tokens, passwords, or other secrets to MEMORY.md, HEARTBEAT.md, AGENTS.md, or normal workspace files.\n")
	builder.WriteString("- Use dedicated secret tools to store, list, or delete named secrets.\n")
	builder.WriteString("Reminders:\n")
	builder.WriteString("- Use dedicated reminder tools for scheduled reminders instead of storing due times in MEMORY.md.\n")
	return strings.TrimSpace(builder.String())
}

func gatewayOverlayPromptBlock() string {
	return defaultHarnessPromptBlock()
}

func stripHTMLComments(text string) string {
	lines := strings.Split(text, "\n")
	out := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "<!--") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
