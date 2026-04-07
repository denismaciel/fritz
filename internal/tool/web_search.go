package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const webSearchModelID = "gemini-2.5-flash"

type WebSearcher interface {
	Search(ctx context.Context, query string) (WebSearchResult, error)
}

type WebSearchResult struct {
	Text    string
	Sources []WebSearchSource
}

type WebSearchSource struct {
	Title string
	URL   string
}

type webSearchTool struct {
	searcher WebSearcher
}

type geminiWebSearcher struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

func NewWebSearchTool(apiKey string, endpoint string) Tool {
	return webSearchTool{
		searcher: geminiWebSearcher{
			apiKey:     strings.TrimSpace(apiKey),
			endpoint:   strings.TrimRight(strings.TrimSpace(endpoint), "/"),
			httpClient: http.DefaultClient,
		},
	}
}

func newWebSearchToolWithSearcher(searcher WebSearcher) Tool {
	return webSearchTool{searcher: searcher}
}

func (t webSearchTool) Definition() Definition {
	return Definition{
		Name:          "web_search",
		Description:   "Search the web for current information and return grounded results with sources.",
		PromptSnippet: "Search the web for current facts, news, docs, and time-sensitive information",
		PromptGuidelines: []string{
			"Use web_search when the answer depends on current or changing information.",
			"Prefer web_search over guessing for news, prices, schedules, current APIs, or changing external facts.",
			"Use returned sources when summarizing web_search results.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"query": {Type: "string", Description: "Search query"},
			},
			Required: []string{"query"},
		},
	}
}

func (t webSearchTool) Run(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	query, errResult, err := requireStringArg("", call, "query")
	if err != nil {
		return errResult, err
	}
	if t.searcher == nil {
		err := errors.New("web search unavailable")
		return errorResult(call, err), err
	}
	result, err := t.searcher.Search(ctx, query)
	if err != nil {
		return errorResult(call, err), err
	}
	text := strings.TrimSpace(result.Text)
	if len(result.Sources) > 0 {
		var builder strings.Builder
		if text != "" {
			builder.WriteString(text)
			builder.WriteString("\n\n")
		}
		builder.WriteString("Sources:\n")
		for _, source := range result.Sources {
			line := "- "
			if strings.TrimSpace(source.Title) != "" {
				line += strings.TrimSpace(source.Title) + ": "
			}
			line += strings.TrimSpace(source.URL)
			builder.WriteString(line)
			builder.WriteString("\n")
		}
		text = strings.TrimSpace(builder.String())
	}
	if text == "" {
		text = "No web results."
	}
	return TextResult(call, text), nil
}

func (s geminiWebSearcher) Search(ctx context.Context, query string) (WebSearchResult, error) {
	if strings.TrimSpace(s.apiKey) == "" {
		return WebSearchResult{}, errors.New("gemini api key missing")
	}
	endpoint := s.endpoint
	if endpoint == "" {
		endpoint = "https://generativelanguage.googleapis.com"
	}
	if s.httpClient == nil {
		s.httpClient = http.DefaultClient
	}
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": query},
				},
			},
		},
		"tools": []map[string]any{
			{"google_search": map[string]any{}},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WebSearchResult{}, err
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", endpoint, webSearchModelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return WebSearchResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-goog-api-key", s.apiKey)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return WebSearchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return WebSearchResult{}, fmt.Errorf("gemini web search failed: %s", strings.TrimSpace(string(data)))
	}
	var payloadResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			GroundingMetadata struct {
				GroundingChunks []struct {
					Web *struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					} `json:"web,omitempty"`
				} `json:"groundingChunks"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return WebSearchResult{}, err
	}
	var out strings.Builder
	seen := map[string]struct{}{}
	var sources []WebSearchSource
	for _, candidate := range payloadResp.Candidates {
		for _, part := range candidate.Content.Parts {
			out.WriteString(part.Text)
		}
		for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
			if chunk.Web == nil {
				continue
			}
			url := strings.TrimSpace(chunk.Web.URI)
			if url == "" {
				continue
			}
			if _, ok := seen[url]; ok {
				continue
			}
			seen[url] = struct{}{}
			sources = append(sources, WebSearchSource{
				Title: strings.TrimSpace(chunk.Web.Title),
				URL:   url,
			})
		}
	}
	text := strings.TrimSpace(out.String())
	if text == "" && len(sources) == 0 {
		return WebSearchResult{}, errors.New("gemini web search empty")
	}
	return WebSearchResult{Text: text, Sources: sources}, nil
}
