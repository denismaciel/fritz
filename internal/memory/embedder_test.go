package memory

import (
	"testing"

	"fritz/internal/config"
)

func TestNewGeminiEmbedderUsesRuntimeConfig(t *testing.T) {
	client, err := NewGeminiEmbedder(config.Runtime{
		GeminiAPIKey:             "test-key",
		GeminiEndpoint:           "https://example.test",
		GeminiEmbeddingModelID:   "gemini-embedding-001",
		GeminiEmbeddingDimension: 256,
	})
	if err != nil {
		t.Fatalf("NewGeminiEmbedder() error = %v", err)
	}
	if client.Name() != "gemini:gemini-embedding-001" {
		t.Fatalf("Name() = %q", client.Name())
	}
	if client.Dim() != 256 {
		t.Fatalf("Dim() = %d", client.Dim())
	}
}
