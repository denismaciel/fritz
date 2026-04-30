package memory

import (
	"fmt"
	"strings"

	"fritz/internal/config"
	mpgemini "fritz/pkg/memorypalace/gemini"
)

func NewGeminiEmbedder(runtime config.Runtime, opts ...mpgemini.Option) (*mpgemini.Client, error) {
	if strings.TrimSpace(runtime.GeminiAPIKey) == "" {
		return nil, fmt.Errorf("missing GEMINI_API_KEY")
	}
	options := []mpgemini.Option{
		mpgemini.WithEndpoint(runtime.GeminiEndpoint),
	}
	if strings.TrimSpace(runtime.GeminiEmbeddingModelID) != "" {
		options = append(options, mpgemini.WithModel(runtime.GeminiEmbeddingModelID))
	}
	if runtime.GeminiEmbeddingDimension > 0 {
		options = append(options, mpgemini.WithOutputDimensionality(runtime.GeminiEmbeddingDimension))
	}
	options = append(options, opts...)
	return mpgemini.New(runtime.GeminiAPIKey, options...)
}
