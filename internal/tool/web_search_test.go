package tool

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchToolFormatsTextAndSources(t *testing.T) {
	tool := newWebSearchToolWithSearcher(fakeWebSearcher{
		result: WebSearchResult{
			Text: "Fresh answer",
			Sources: []WebSearchSource{
				{Title: "One", URL: "https://example.com/1"},
				{Title: "Two", URL: "https://example.com/2"},
			},
		},
	})

	result, err := tool.Run(context.Background(), Call{
		ID:   "1",
		Name: "web_search",
		Args: map[string]any{"query": "current thing"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	text := result.Text()
	if !strings.Contains(text, "Fresh answer") || !strings.Contains(text, "https://example.com/1") {
		t.Fatalf("text = %q", text)
	}
}

func TestGeminiWebSearcherParsesGroundedResponse(t *testing.T) {
	var sawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		sawBody = string(data)
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{
			"candidates": [{
				"content": {"parts": [{"text": "Current answer"}]},
				"groundingMetadata": {
					"groundingChunks": [
						{"web": {"uri": "https://example.com/a", "title": "A"}},
						{"web": {"uri": "https://example.com/b", "title": "B"}},
						{"web": {"uri": "https://example.com/a", "title": "A"}}
					]
				}
			}]
		}`)
	}))
	defer server.Close()

	searcher := geminiWebSearcher{
		apiKey:     "test-key",
		endpoint:   server.URL,
		httpClient: server.Client(),
	}
	result, err := searcher.Search(context.Background(), "news now")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result.Text != "Current answer" {
		t.Fatalf("Text = %q", result.Text)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("Sources = %#v", result.Sources)
	}
	if !strings.Contains(sawBody, `"google_search":{}`) {
		t.Fatalf("body = %s", sawBody)
	}
}

type fakeWebSearcher struct {
	result WebSearchResult
	err    error
}

func (f fakeWebSearcher) Search(context.Context, string) (WebSearchResult, error) {
	return f.result, f.err
}
