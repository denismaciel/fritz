package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"fritz/internal/config"
)

type HTTPClient struct {
	client    *http.Client
	endpoint  string
	botToken  string
	appToken  string
}

type apiResponse[T any] struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	URL   string `json:"url,omitempty"`
	FileID string `json:"file_id,omitempty"`
	TS    string `json:"ts,omitempty"`
	Files []struct {
		ID string `json:"id,omitempty"`
	} `json:"files,omitempty"`
	Messages []HistoryMessage `json:"messages,omitempty"`
}

func NewHTTPClient(httpClient *http.Client, endpoint string, botToken string, appToken string) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = config.DefaultSlackAPIEndpoint
	}
	return &HTTPClient{
		client:   httpClient,
		endpoint: strings.TrimRight(endpoint, "/"),
		botToken: strings.TrimSpace(botToken),
		appToken: strings.TrimSpace(appToken),
	}
}

func (c *HTTPClient) OpenSocketConnection(ctx context.Context) (*SocketConn, error) {
	var response apiResponse[struct{}]
	if err := c.post(ctx, c.appToken, "apps.connections.open", map[string]any{}, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.URL) == "" {
		return nil, fmt.Errorf("slack socket url missing")
	}
	return DialSocket(ctx, response.URL)
}

func (c *HTTPClient) PostMessage(ctx context.Context, req PostMessageRequest) error {
	var response apiResponse[json.RawMessage]
	return c.post(ctx, c.botToken, "chat.postMessage", req, &response)
}

func (c *HTTPClient) StartStream(ctx context.Context, channelID string, threadTS string) (StreamHandle, error) {
	payload := map[string]any{
		"channel": channelID,
	}
	if strings.TrimSpace(threadTS) != "" {
		payload["thread_ts"] = threadTS
	}
	var response apiResponse[json.RawMessage]
	if err := c.post(ctx, c.botToken, "chat.startStream", payload, &response); err != nil {
		return StreamHandle{}, err
	}
	if strings.TrimSpace(response.TS) == "" {
		return StreamHandle{}, fmt.Errorf("slack stream ts missing")
	}
	return StreamHandle{Channel: channelID, ThreadTS: threadTS, TS: response.TS}, nil
}

func (c *HTTPClient) AppendStream(ctx context.Context, handle StreamHandle, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var response apiResponse[json.RawMessage]
	return c.post(ctx, c.botToken, "chat.appendStream", map[string]any{
		"channel":       handle.Channel,
		"ts":            handle.TS,
		"markdown_text": text,
	}, &response)
}

func (c *HTTPClient) StopStream(ctx context.Context, handle StreamHandle) error {
	var response apiResponse[json.RawMessage]
	return c.post(ctx, c.botToken, "chat.stopStream", map[string]any{
		"channel": handle.Channel,
		"ts":      handle.TS,
	}, &response)
}

func (c *HTTPClient) SetStatus(ctx context.Context, ref AssistantThreadRef, status string) error {
	if strings.TrimSpace(ref.ChannelID) == "" || strings.TrimSpace(ref.ThreadTS) == "" {
		return nil
	}
	var response apiResponse[json.RawMessage]
	return c.post(ctx, c.botToken, "assistant.threads.setStatus", map[string]any{
		"channel_id": ref.ChannelID,
		"thread_ts":  ref.ThreadTS,
		"status":     status,
	}, &response)
}

func (c *HTTPClient) SetTitle(ctx context.Context, ref AssistantThreadRef, title string) error {
	if strings.TrimSpace(ref.ChannelID) == "" || strings.TrimSpace(ref.ThreadTS) == "" || strings.TrimSpace(title) == "" {
		return nil
	}
	var response apiResponse[json.RawMessage]
	return c.post(ctx, c.botToken, "assistant.threads.setTitle", map[string]any{
		"channel_id": ref.ChannelID,
		"thread_ts":  ref.ThreadTS,
		"title":      title,
	}, &response)
}

func (c *HTTPClient) SetSuggestedPrompts(ctx context.Context, ref AssistantThreadRef, prompts []SuggestedPrompt) error {
	if strings.TrimSpace(ref.ChannelID) == "" || strings.TrimSpace(ref.ThreadTS) == "" || len(prompts) == 0 {
		return nil
	}
	var response apiResponse[json.RawMessage]
	return c.post(ctx, c.botToken, "assistant.threads.setSuggestedPrompts", map[string]any{
		"channel_id": ref.ChannelID,
		"thread_ts":  ref.ThreadTS,
		"prompts":    prompts,
	}, &response)
}

func (c *HTTPClient) ConversationsReplies(ctx context.Context, channelID string, ts string) ([]HistoryMessage, error) {
	var response apiResponse[json.RawMessage]
	if err := c.post(ctx, c.botToken, "conversations.replies", map[string]any{
		"channel": channelID,
		"ts":      ts,
	}, &response); err != nil {
		return nil, err
	}
	return response.Messages, nil
}

func (c *HTTPClient) UploadFile(ctx context.Context, req UploadFileRequest) error {
	var ticket apiResponse[json.RawMessage]
	if err := c.post(ctx, c.botToken, "files.getUploadURLExternal", map[string]any{
		"filename": req.Filename,
		"length":   len(req.Bytes),
	}, &ticket); err != nil {
		return err
	}
	if strings.TrimSpace(ticket.URL) == "" {
		return fmt.Errorf("slack upload url missing")
	}
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ticket.URL, bytes.NewReader(req.Bytes))
	if err != nil {
		return err
	}
	uploadReq.Header.Set("content-type", "application/octet-stream")
	uploadReq.Header.Set("content-length", fmt.Sprintf("%d", len(req.Bytes)))
	uploadResp, err := c.client.Do(uploadReq)
	if err != nil {
		return err
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode >= 300 {
		body, _ := io.ReadAll(uploadResp.Body)
		return fmt.Errorf("slack upload failed: %s", strings.TrimSpace(string(body)))
	}
	fileID := strings.TrimSpace(ticket.FileID)
	if fileID == "" {
		return fmt.Errorf("slack file id missing")
	}
	var complete apiResponse[json.RawMessage]
	return c.post(ctx, c.botToken, "files.completeUploadExternal", map[string]any{
		"files": []map[string]any{{
			"id":    fileID,
			"title": req.Title,
		}},
		"channel_id": req.ChannelID,
		"thread_ts":  req.ThreadTS,
	}, &complete)
}

func (c *HTTPClient) post(ctx context.Context, token string, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")
	req.Header.Set("authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack %s failed: %s", method, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	switch typed := out.(type) {
	case *apiResponse[json.RawMessage]:
		if !typed.OK {
			return fmt.Errorf("slack %s failed: %s", method, strings.TrimSpace(typed.Error))
		}
	case *apiResponse[struct{}]:
		if !typed.OK {
			return fmt.Errorf("slack %s failed: %s", method, strings.TrimSpace(typed.Error))
		}
	}
	return nil
}
