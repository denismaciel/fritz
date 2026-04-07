package provider

import (
	"fmt"
	"strings"
)

type Kind string

const (
	Gemini      Kind = "gemini"
	OpenAICodex Kind = "openai-codex"
)

func Parse(value string) (Kind, error) {
	kind := Kind(strings.TrimSpace(value))
	switch kind {
	case "", Gemini:
		return Gemini, nil
	case OpenAICodex:
		return OpenAICodex, nil
	default:
		return "", fmt.Errorf("unsupported provider %q", value)
	}
}

func MustParse(value string) Kind {
	kind, err := Parse(value)
	if err != nil {
		panic(err)
	}
	return kind
}

