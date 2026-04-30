package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"fritz/pkg/memorypalace"
)

const (
	DefaultEndpoint = "https://generativelanguage.googleapis.com"
	DefaultModel    = "gemini-embedding-001"
)

type Client struct {
	apiKey               string
	endpoint             string
	model                string
	httpClient           *http.Client
	outputDimensionality int
	dim                  int
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		if strings.TrimSpace(endpoint) != "" {
			c.endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

func WithModel(model string) Option {
	return func(c *Client) {
		if strings.TrimSpace(model) != "" {
			c.model = strings.TrimSpace(model)
		}
	}
}

func WithOutputDimensionality(dim int) Option {
	return func(c *Client) {
		if dim > 0 {
			c.outputDimensionality = dim
			c.dim = dim
		}
	}
}

func New(apiKey string, opts ...Option) (*Client, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%w: gemini api key required", memorypalace.ErrInvalidRequest)
	}
	client := &Client{
		apiKey:     apiKey,
		endpoint:   DefaultEndpoint,
		model:      DefaultModel,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

func (c *Client) Name() string {
	return "gemini:" + c.model
}

func (c *Client) Dim() int {
	return c.dim
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type embedRequest struct {
		Model                string  `json:"model"`
		Content              content `json:"content"`
		OutputDimensionality int     `json:"outputDimensionality,omitempty"`
	}
	requests := make([]embedRequest, 0, len(texts))
	for _, text := range texts {
		request := embedRequest{
			Model:   "models/" + c.model,
			Content: content{Parts: []part{{Text: text}}},
		}
		if c.outputDimensionality > 0 {
			request.OutputDimensionality = c.outputDimensionality
		}
		requests = append(requests, request)
	}
	body, err := json.Marshal(map[string]any{
		"requests": requests,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urlFor("batchEmbedContents"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("memorypalace: gemini embed failed: %s", strings.TrimSpace(string(data)))
	}

	var payload struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([][]float32, 0, len(payload.Embeddings))
	for _, embedding := range payload.Embeddings {
		out = append(out, embedding.Values)
	}
	if c.dim == 0 && len(out) > 0 {
		c.dim = len(out[0])
	}
	return out, nil
}

func (c *Client) urlFor(method string) string {
	return fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(c.endpoint, "/"), c.model, method)
}
