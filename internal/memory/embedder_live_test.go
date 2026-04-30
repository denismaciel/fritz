package memory

import (
	"context"
	"os"
	"testing"

	"fritz/internal/config"
)

func TestLiveGeminiEmbedder(t *testing.T) {
	if os.Getenv("FRITZ_LIVE_GEMINI_EMBED_TEST") != "1" {
		t.Skip("set FRITZ_LIVE_GEMINI_EMBED_TEST=1 to run live Gemini embed smoke test")
	}

	runtime := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Env:      config.LoadEnv(),
	})
	if runtime.GeminiAPIKey == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	embedder, err := NewGeminiEmbedder(runtime)
	if err != nil {
		t.Fatalf("NewGeminiEmbedder() error = %v", err)
	}

	vectors, err := embedder.Embed(context.Background(), []string{
		"fritz memory palace smoke test",
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("vectors = %#v", vectors)
	}
	if len(vectors[0]) == 0 {
		t.Fatalf("vector length = %d", len(vectors[0]))
	}

	sample := vectors[0]
	if len(sample) > 3 {
		sample = sample[:3]
	}
	t.Logf("model=%s dim=%d sample=%v", embedder.Name(), len(vectors[0]), sample)
}
