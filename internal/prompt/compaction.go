package prompt

import (
	_ "embed"
	"strings"

	"fritz/internal/chat"
)

var (
	//go:embed reference/local/compaction_prompt.md
	localCompactionPrompt string

	//go:embed reference/local/compaction_summary_prefix.md
	localCompactionSummaryPrefix string
)

func CompactionPromptTemplate() string {
	return strings.TrimSpace(localCompactionPrompt)
}

func CompactionSummaryPrefix() string {
	return strings.TrimSpace(localCompactionSummaryPrefix)
}

func BuildCompactionPrompt(turns chat.Transcript, customInstructions string) string {
	var builder strings.Builder
	builder.WriteString(CompactionPromptTemplate())
	if customInstructions = strings.TrimSpace(customInstructions); customInstructions != "" {
		builder.WriteString("\n\nAdditional instructions:\n")
		builder.WriteString(customInstructions)
	}
	if len(turns) > 0 {
		builder.WriteString("\n\nTranscript to compact:\n")
		for _, turn := range turns {
			builder.WriteString("User: ")
			builder.WriteString(strings.TrimSpace(turn.User))
			builder.WriteString("\nAssistant: ")
			builder.WriteString(strings.TrimSpace(turn.Assistant))
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func BuildCompactionSummaryMessage(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return CompactionSummaryPrefix()
	}
	return CompactionSummaryPrefix() + "\n\n" + summary
}

func IsCompactionSummaryMessage(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), CompactionSummaryPrefix())
}
