package openaicodex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"fritz/internal/model"
	"fritz/internal/provider"
	"fritz/internal/tool"
)

const defaultEndpoint = "https://chatgpt.com/backend-api"

type AuthFunc func(context.Context) (provider.RequestAuth, error)
type Option func(*Client)

type Client struct {
	resolveAuth AuthFunc
	endpoint    string
	model       string
	httpClient  *http.Client
}

type requestBody struct {
	Model             string                 `json:"model"`
	Store             bool                   `json:"store"`
	Stream            bool                   `json:"stream"`
	Instructions      string                 `json:"instructions,omitempty"`
	Input             []any                  `json:"input,omitempty"`
	Tools             []responseTool         `json:"tools,omitempty"`
	ToolChoice        string                 `json:"tool_choice,omitempty"`
	ParallelToolCalls bool                   `json:"parallel_tool_calls,omitempty"`
	Text              responseTextOpts       `json:"text,omitempty"`
	Reasoning         *responseReasoningOpts `json:"reasoning,omitempty"`
	Include           []string               `json:"include,omitempty"`
	ServiceTier       string                 `json:"service_tier,omitempty"`
}

type responseTextOpts struct {
	Verbosity string `json:"verbosity,omitempty"`
}

type responseReasoningOpts struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type responseTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type partialToolCall struct {
	id           string
	callID       string
	name         string
	argumentsRaw string
}

func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		c.endpoint = strings.TrimRight(endpoint, "/")
	}
}

func WithModel(modelID string) Option {
	return func(c *Client) {
		c.model = modelID
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func NewClient(resolveAuth AuthFunc, options ...Option) Client {
	client := Client{
		resolveAuth: resolveAuth,
		endpoint:    defaultEndpoint,
		httpClient:  http.DefaultClient,
	}
	for _, option := range options {
		option(&client)
	}
	return client
}

func (c Client) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	return c.StreamGenerate(ctx, req, func(model.StreamEvent) error { return nil })
}

func (c Client) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	if c.resolveAuth == nil {
		return model.Response{}, fmt.Errorf("missing auth resolver")
	}
	auth, err := c.resolveAuth(ctx)
	if err != nil {
		return model.Response{}, err
	}
	bodyBytes, err := json.Marshal(buildRequestBody(req, fallbackModel(req.ModelID, c.model)))
	if err != nil {
		return model.Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveCodexURL(c.endpoint), bytes.NewReader(bodyBytes))
	if err != nil {
		return model.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if auth.BearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+auth.BearerToken)
	}
	if auth.AccountID != "" {
		httpReq.Header.Set("chatgpt-account-id", auth.AccountID)
	}
	for key, value := range auth.Headers {
		httpReq.Header.Set(key, value)
	}
	if httpReq.Header.Get("OpenAI-Beta") == "" {
		httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return model.Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return model.Response{}, fmt.Errorf("codex request failed: %s", strings.TrimSpace(string(data)))
	}
	return decodeSSE(resp.Body, emit)
}

func buildRequestBody(req model.Request, modelID string) requestBody {
	baseModel, reasoning := parseModelID(modelID)
	body := requestBody{
		Model:             baseModel,
		Store:             false,
		Stream:            true,
		Instructions:      req.SystemPrompt,
		Input:             buildInput(req.Messages),
		Text:              responseTextOpts{Verbosity: "medium"},
		Reasoning:         reasoning,
		Include:           []string{"reasoning.encrypted_content"},
		ToolChoice:        "auto",
		ParallelToolCalls: true,
		ServiceTier:       "fast",
	}
	if len(req.Tools) > 0 {
		body.Tools = make([]responseTool, 0, len(req.Tools))
		for _, definition := range req.Tools {
			body.Tools = append(body.Tools, responseTool{
				Type:        "function",
				Name:        definition.Name,
				Description: definition.Description,
				Parameters:  normalizeParameters(definition.Parameters),
			})
		}
	}
	return body
}

func normalizeParameters(params tool.Parameters) map[string]any {
	out := map[string]any{
		"type": params.Type,
	}
	properties := map[string]any{}
	for key, value := range params.Properties {
		properties[key] = normalizeProperty(value)
	}
	if params.Type == "object" {
		out["properties"] = properties
	} else if len(properties) > 0 {
		out["properties"] = properties
	}
	if len(params.Required) > 0 {
		out["required"] = append([]string(nil), params.Required...)
	}
	return out
}

func normalizeProperty(prop tool.Property) map[string]any {
	out := map[string]any{
		"type": prop.Type,
	}
	if prop.Description != "" {
		out["description"] = prop.Description
	}
	properties := map[string]any{}
	for key, value := range prop.Properties {
		properties[key] = normalizeProperty(value)
	}
	if prop.Type == "object" {
		out["properties"] = properties
	} else if len(properties) > 0 {
		out["properties"] = properties
	}
	if len(prop.Required) > 0 {
		out["required"] = append([]string(nil), prop.Required...)
	}
	if prop.Items != nil {
		out["items"] = normalizeProperty(*prop.Items)
	}
	return out
}

func buildInput(messages []model.Message) []any {
	out := make([]any, 0, len(messages))
	for index, msg := range messages {
		switch msg.Role {
		case model.UserRole:
			userParts := make([]map[string]any, 0, len(msg.Parts))
			for _, part := range msg.Parts {
				switch {
				case part.ToolResult != nil:
					out = append(out, map[string]any{
						"type":    "function_call_output",
						"call_id": part.ToolResult.CallID,
						"output":  buildToolOutput(*part.ToolResult),
					})
				case part.Text != "" && !part.Thought:
					userParts = append(userParts, map[string]any{
						"type": "input_text",
						"text": part.Text,
					})
				}
			}
			if len(userParts) > 0 {
				out = append(out, map[string]any{
					"role":    "user",
					"content": userParts,
				})
			}
		case model.ModelRole:
			assistantParts := make([]map[string]any, 0, len(msg.Parts))
			for _, part := range msg.Parts {
				switch {
				case part.Text != "" && !part.Thought:
					assistantParts = append(assistantParts, map[string]any{
						"type":        "output_text",
						"text":        part.Text,
						"annotations": []any{},
					})
				case part.ToolCall != nil:
					args, _ := json.Marshal(part.ToolCall.Args)
					out = append(out, map[string]any{
						"type":      "function_call",
						"id":        toolCallItemID(part.ToolCall.ID),
						"call_id":   part.ToolCall.ID,
						"name":      part.ToolCall.Name,
						"arguments": string(args),
					})
				}
			}
			if len(assistantParts) > 0 {
				out = append(out, map[string]any{
					"type":    "message",
					"id":      fmt.Sprintf("msg_%d", index),
					"role":    "assistant",
					"status":  "completed",
					"content": assistantParts,
				})
			}
		}
	}
	return out
}

func buildToolOutput(result tool.Result) any {
	if text := strings.TrimSpace(result.Text()); text != "" {
		return text
	}
	if image, ok := result.FirstImage(); ok {
		return []map[string]any{{
			"type":      "input_image",
			"image_url": "data:" + image.MIMEType + ";base64," + image.Data,
			"detail":    "auto",
		}}
	}
	return "(empty tool result)"
}

func toolCallItemID(callID string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '_' || r == '-':
			return r
		default:
			return '_'
		}
	}, callID)
	if !strings.HasPrefix(sanitized, "fc_") {
		sanitized = "fc_" + sanitized
	}
	if len(sanitized) > 64 {
		sanitized = sanitized[:64]
	}
	return sanitized
}

func resolveCodexURL(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	switch {
	case strings.HasSuffix(base, "/codex/responses"):
		return base
	case strings.HasSuffix(base, "/codex"):
		return base + "/responses"
	default:
		return base + "/codex/responses"
	}
}

func decodeSSE(body io.Reader, emit func(model.StreamEvent) error) (model.Response, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var chunk []string
	response := model.Response{Message: model.Message{Role: model.ModelRole}}
	var textBuf strings.Builder
	var reasoningBuf strings.Builder
	var currentTool *partialToolCall
	var currentReasoning bool
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := consumeSSEChunk(chunk, emit, &response, &textBuf, &reasoningBuf, &currentTool, &currentReasoning); err != nil {
				return model.Response{}, err
			}
			chunk = chunk[:0]
			continue
		}
		chunk = append(chunk, line)
	}
	if len(chunk) > 0 {
		if err := consumeSSEChunk(chunk, emit, &response, &textBuf, &reasoningBuf, &currentTool, &currentReasoning); err != nil {
			return model.Response{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return model.Response{}, err
	}
	response.Text = textBuf.String()
	if response.Text == "" {
		response.Text = response.Message.Text()
	}
	if reasoningBuf.Len() > 0 {
		response.Message.Parts = append([]model.Part{{Text: reasoningBuf.String(), Thought: true}}, response.Message.Parts...)
	}
	return response, nil
}

func consumeSSEChunk(
	lines []string,
	emit func(model.StreamEvent) error,
	response *model.Response,
	textBuf *strings.Builder,
	reasoningBuf *strings.Builder,
	currentTool **partialToolCall,
	currentReasoning *bool,
) error {
	if len(lines) == 0 {
		return nil
	}
	var dataLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) == 0 {
		return nil
	}
	payload := strings.Join(dataLines, "\n")
	if payload == "[DONE]" {
		return nil
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return err
	}
	eventType, _ := event["type"].(string)
	switch eventType {
	case "error":
		return fmt.Errorf("codex error: %v", event["message"])
	case "response.failed":
		if resp, _ := event["response"].(map[string]any); resp != nil {
			if errPayload, _ := resp["error"].(map[string]any); errPayload != nil {
				if message, _ := errPayload["message"].(string); message != "" {
					return errorsNew(message)
				}
			}
		}
		return errorsNew("codex response failed")
	case "response.output_item.added":
		item, _ := event["item"].(map[string]any)
		itemType, _ := item["type"].(string)
		switch itemType {
		case "reasoning":
			*currentReasoning = true
		case "function_call":
			*currentTool = &partialToolCall{
				id:           stringValue(item["id"]),
				callID:       stringValue(item["call_id"]),
				name:         stringValue(item["name"]),
				argumentsRaw: stringValue(item["arguments"]),
			}
		}
	case "response.reasoning_summary_text.delta":
		delta := stringValue(event["delta"])
		reasoningBuf.WriteString(delta)
		if emit != nil {
			if err := emit(model.StreamEvent{ReasoningDelta: delta}); err != nil {
				return err
			}
		}
	case "response.reasoning_summary_part.done":
		reasoningBuf.WriteString("\n\n")
		if emit != nil {
			if err := emit(model.StreamEvent{ReasoningDelta: "\n\n"}); err != nil {
				return err
			}
		}
	case "response.output_text.delta", "response.refusal.delta":
		delta := stringValue(event["delta"])
		textBuf.WriteString(delta)
		if emit != nil {
			if err := emit(model.StreamEvent{TextDelta: delta}); err != nil {
				return err
			}
		}
	case "response.function_call_arguments.delta":
		if *currentTool != nil {
			(*currentTool).argumentsRaw += stringValue(event["delta"])
		}
	case "response.function_call_arguments.done":
		if *currentTool != nil {
			if arguments := stringValue(event["arguments"]); arguments != "" {
				(*currentTool).argumentsRaw = arguments
			}
		}
	case "response.output_item.done":
		item, _ := event["item"].(map[string]any)
		itemType, _ := item["type"].(string)
		switch itemType {
		case "message":
			text := joinedOutputText(item)
			if text != "" {
				response.Message.Parts = append(response.Message.Parts, model.Part{Text: text})
			}
		case "function_call":
			call := partialToolFromItem(item, *currentTool)
			response.Message.Parts = append(response.Message.Parts, model.Part{ToolCall: &call})
			response.ToolCalls = append(response.ToolCalls, call)
			*currentTool = nil
		case "reasoning":
			*currentReasoning = false
		}
	}
	return nil
}

func partialToolFromItem(item map[string]any, current *partialToolCall) tool.Call {
	call := tool.Call{
		ID:   stringValue(item["call_id"]),
		Name: stringValue(item["name"]),
		Args: map[string]any{},
	}
	raw := stringValue(item["arguments"])
	if current != nil {
		if call.ID == "" {
			call.ID = current.callID
		}
		if call.Name == "" {
			call.Name = current.name
		}
		if raw == "" {
			raw = current.argumentsRaw
		}
	}
	_ = json.Unmarshal([]byte(raw), &call.Args)
	return call
}

func joinedOutputText(item map[string]any) string {
	content, _ := item["content"].([]any)
	var out strings.Builder
	for _, raw := range content {
		part, _ := raw.(map[string]any)
		partType, _ := part["type"].(string)
		switch partType {
		case "output_text":
			out.WriteString(stringValue(part["text"]))
		case "refusal":
			out.WriteString(stringValue(part["refusal"]))
		}
	}
	return out.String()
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func fallbackModel(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "gpt-5.4"
}

func parseModelID(id string) (string, *responseReasoningOpts) {
	if id == "" {
		return "", nil
	}

	efforts := []string{"minimal", "low", "medium", "high", "xhigh"}
	for _, effort := range efforts {
		suffix := "-" + effort + "-reasoning"
		if strings.Contains(id, suffix) {
			return strings.ReplaceAll(id, suffix, ""), &responseReasoningOpts{
				Effort:  effort,
				Summary: "auto",
			}
		}
	}

	return id, nil
}

func errorsNew(message string) error {
	return fmt.Errorf("%s", message)
}
