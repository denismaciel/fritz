package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"fritz/internal/logx"
	"fritz/internal/model"
	"fritz/internal/tool"
)

const (
	defaultEndpoint = "https://generativelanguage.googleapis.com"
	defaultModel    = "gemini-3-flash-preview"
	maxSSELineBytes = 64 * 1024 * 1024
)

type Client struct {
	apiKey           string
	endpoint         string
	model            string
	httpClient       *http.Client
	retryMaxAttempts int
	retryBaseDelay   time.Duration
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		c.endpoint = strings.TrimRight(endpoint, "/")
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func WithModel(model string) Option {
	return func(c *Client) {
		c.model = model
	}
}

func WithRetryPolicy(maxAttempts int, baseDelay time.Duration) Option {
	return func(c *Client) {
		if maxAttempts > 0 {
			c.retryMaxAttempts = maxAttempts
		}
		if baseDelay > 0 {
			c.retryBaseDelay = baseDelay
		}
	}
}

func NewClient(apiKey string, options ...Option) Client {
	client := Client{
		apiKey:           apiKey,
		endpoint:         defaultEndpoint,
		model:            defaultModel,
		httpClient:       http.DefaultClient,
		retryMaxAttempts: 3,
		retryBaseDelay:   200 * time.Millisecond,
	}

	for _, option := range options {
		option(&client)
	}

	return client
}

func (c Client) Provider() string {
	return "gemini"
}

func (c Client) Endpoint() string {
	return c.endpoint
}

func (c Client) APIKey() string {
	return c.apiKey
}

func (c Client) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	logger := logx.FromContext(ctx).With().
		Str("component", "model").
		Str("event", "model.generate").
		Str("provider", c.Provider()).
		Str("model", firstNonBlank(req.ModelID, c.model)).
		Int("messages", len(req.Messages)).
		Int("tools", len(req.Tools)).
		Logger()
	bodyBytes, err := json.Marshal(buildRequest(req))
	if err != nil {
		logger.Error().Err(err).Str("stage", "marshal").Msg("")
		return model.Response{}, err
	}

	var lastErr error
	for attempt := 1; attempt <= c.retryMaxAttempts; attempt++ {
		start := time.Now()
		response, err := c.generateOnce(ctx, req, bodyBytes)
		if err == nil {
			logger.Info().Int("attempt", attempt).Dur("duration", time.Since(start)).Int("tool_calls", len(response.ToolCalls)).Int("text_len", len(strings.TrimSpace(response.Message.Text()))).Msg("")
			return response, nil
		}
		lastErr = err
		logger.Warn().Err(err).Int("attempt", attempt).Dur("duration", time.Since(start)).Msg("")
		if !shouldRetry(err) || attempt == c.retryMaxAttempts {
			return model.Response{}, err
		}
		if err := sleepWithContext(ctx, time.Duration(attempt)*c.retryBaseDelay); err != nil {
			return model.Response{}, err
		}
	}
	return model.Response{}, lastErr
}

func (c Client) generateOnce(ctx context.Context, req model.Request, bodyBytes []byte) (model.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urlFor(req.ModelID, "generateContent"), bytes.NewReader(bodyBytes))
	if err != nil {
		return model.Response{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return model.Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return model.Response{}, newRequestError("generate", resp)
	}

	var payload generateContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return model.Response{}, err
	}
	return convertResponse(payload)
}

func (c Client) StreamGenerate(
	ctx context.Context,
	req model.Request,
	emit func(model.StreamEvent) error,
) (model.Response, error) {
	logger := logx.FromContext(ctx).With().
		Str("component", "model").
		Str("event", "model.stream").
		Str("provider", c.Provider()).
		Str("model", firstNonBlank(req.ModelID, c.model)).
		Int("messages", len(req.Messages)).
		Int("tools", len(req.Tools)).
		Logger()
	bodyBytes, err := json.Marshal(buildRequest(req))
	if err != nil {
		logger.Error().Err(err).Str("stage", "marshal").Msg("")
		return model.Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.urlFor(req.ModelID, "streamGenerateContent"), bytes.NewReader(bodyBytes))
	if err != nil {
		return model.Response{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)
	query := httpReq.URL.Query()
	query.Set("alt", "sse")
	httpReq.URL.RawQuery = query.Encode()

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		logger.Error().Err(err).Str("stage", "request").Msg("")
		return model.Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		err := newRequestError("stream", resp)
		logger.Error().Err(err).Str("stage", "status").Int("status", resp.StatusCode).Msg("")
		return model.Response{}, err
	}
	response, err := decodeSSE(resp.Body, emit)
	if err != nil {
		logger.Error().Err(err).Str("stage", "decode").Msg("")
		return model.Response{}, err
	}
	logger.Info().Int("tool_calls", len(response.ToolCalls)).Int("text_len", len(strings.TrimSpace(response.Message.Text()))).Msg("")
	return response, nil
}

func (c Client) urlFor(modelID string, method string) string {
	if modelID == "" {
		modelID = c.model
	}
	return fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(c.endpoint, "/"), modelID, method)
}

type requestBody struct {
	SystemInstruction *content          `json:"system_instruction,omitempty"`
	Contents          []content         `json:"contents"`
	Tools             []apiTool         `json:"tools,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
}

type generationConfig struct {
	ThinkingConfig *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type thinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

type content struct {
	Role  string    `json:"role"`
	Parts []apiPart `json:"parts"`
}

type apiTool struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations"`
}

type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  tool.Parameters `json:"parameters"`
}

type apiPart struct {
	Text             string               `json:"text,omitempty"`
	InlineData       *apiInlineData       `json:"inline_data,omitempty"`
	Thought          bool                 `json:"thought,omitempty"`
	FunctionCall     *apiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *apiFunctionResponse `json:"functionResponse,omitempty"`
	ThoughtSignature string               `json:"thoughtSignature,omitempty"`
}

type apiInlineData struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
}

type apiFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type apiFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

func buildRequest(req model.Request) requestBody {
	body := requestBody{
		Contents: make([]content, 0, len(req.Messages)),
		GenerationConfig: &generationConfig{
			ThinkingConfig: &thinkingConfig{
				IncludeThoughts: true,
			},
		},
	}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		body.SystemInstruction = &content{
			Parts: []apiPart{{Text: req.SystemPrompt}},
		}
	}

	for _, message := range req.Messages {
		parts := make([]apiPart, 0, len(message.Parts))
		for _, part := range message.Parts {
			api := apiPart{
				Text:             part.Text,
				Thought:          part.Thought,
				ThoughtSignature: part.ThoughtSignature,
			}
			if part.ImageData != "" {
				api.InlineData = &apiInlineData{
					MIMEType: part.ImageMIMEType,
					Data:     part.ImageData,
				}
			}
			if part.ToolCall != nil {
				api.FunctionCall = &apiFunctionCall{
					ID:   part.ToolCall.ID,
					Name: part.ToolCall.Name,
					Args: part.ToolCall.Args,
				}
			}
			if part.ToolResult != nil {
				response := map[string]any{
					"parts":    part.ToolResult.Parts,
					"is_error": part.ToolResult.IsError,
				}
				if part.ToolResult.Details != nil {
					response["details"] = part.ToolResult.Details
				}
				api.FunctionResponse = &apiFunctionResponse{
					ID:       part.ToolResult.CallID,
					Name:     part.ToolResult.Name,
					Response: response,
				}
			}
			parts = append(parts, api)
		}
		body.Contents = append(body.Contents, content{
			Role:  string(message.Role),
			Parts: parts,
		})
	}

	if len(req.Tools) > 0 {
		declarations := make([]functionDeclaration, 0, len(req.Tools))
		for _, def := range req.Tools {
			declarations = append(declarations, functionDeclaration{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			})
		}
		body.Tools = []apiTool{{FunctionDeclarations: declarations}}
	}

	return body
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type generateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []apiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func convertResponse(payload generateContentResponse) (model.Response, error) {
	if len(payload.Candidates) == 0 {
		return model.Response{}, errors.New("gemini response had no candidates")
	}

	parts := payload.Candidates[0].Content.Parts
	msgParts := make([]model.Part, 0, len(parts))
	text := strings.Builder{}
	toolCalls := make([]tool.Call, 0)

	for _, part := range parts {
		msgPart := model.Part{
			Text:             part.Text,
			Thought:          part.Thought,
			ThoughtSignature: part.ThoughtSignature,
		}
		if part.Text != "" && !part.Thought {
			text.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			call := tool.Call{
				ID:   part.FunctionCall.ID,
				Name: part.FunctionCall.Name,
				Args: part.FunctionCall.Args,
			}
			msgPart.ToolCall = &call
			toolCalls = append(toolCalls, call)
		}
		msgParts = append(msgParts, msgPart)
	}

	if len(msgParts) == 0 {
		return model.Response{}, errors.New("gemini response had no parts")
	}

	return model.Response{
		Message: model.Message{
			Role:  model.ModelRole,
			Parts: msgParts,
		},
		Text:      text.String(),
		ToolCalls: toolCalls,
	}, nil
}

func decodeSSE(body io.Reader, emit func(model.StreamEvent) error) (model.Response, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)

	var finalText strings.Builder
	var toolCalls []tool.Call
	var msgParts []model.Part

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		var chunk generateContentResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return model.Response{}, err
		}

		if len(chunk.Candidates) == 0 {
			continue
		}

		for _, part := range chunk.Candidates[0].Content.Parts {
			msgPart := model.Part{
				Text:             part.Text,
				Thought:          part.Thought,
				ThoughtSignature: part.ThoughtSignature,
			}
			if part.Text != "" && part.Thought {
				if err := emit(model.StreamEvent{ReasoningDelta: part.Text}); err != nil {
					return model.Response{}, err
				}
			}
			if part.Text != "" && !part.Thought {
				finalText.WriteString(part.Text)
				if err := emit(model.StreamEvent{TextDelta: part.Text}); err != nil {
					return model.Response{}, err
				}
			}
			if part.FunctionCall != nil {
				call := tool.Call{
					ID:   part.FunctionCall.ID,
					Name: part.FunctionCall.Name,
					Args: part.FunctionCall.Args,
				}
				msgPart.ToolCall = &call
				toolCalls = append(toolCalls, call)
			}
			if msgPart.Text != "" || msgPart.ToolCall != nil || msgPart.ThoughtSignature != "" {
				msgParts = append(msgParts, msgPart)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return model.Response{}, fmt.Errorf("decode gemini sse stream: %w", err)
	}
	if len(msgParts) == 0 {
		return model.Response{}, errors.New("gemini response had no parts")
	}

	return model.Response{
		Message: model.Message{
			Role:  model.ModelRole,
			Parts: msgParts,
		},
		Text:      finalText.String(),
		ToolCalls: toolCalls,
	}, nil
}

type RequestError struct {
	Operation  string
	StatusCode int
	Status     string
	Body       string
}

func (e *RequestError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("gemini %s request failed: %s: %s", e.Operation, e.Status, e.Body)
	}
	return fmt.Sprintf("gemini %s request failed: %s", e.Operation, e.Status)
}

func newRequestError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return &RequestError{
		Operation:  operation,
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       strings.TrimSpace(string(body)),
	}
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode == http.StatusTooManyRequests || reqErr.StatusCode >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
