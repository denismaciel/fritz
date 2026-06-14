package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"fritz/internal/logx"
)

const DefaultEndpoint = "https://api.telegram.org"

type HTTPClient struct {
	client   *http.Client
	endpoint string
	token    string
}

type File struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
}

type GetUpdatesRequest struct {
	Offset         int64 `json:"offset,omitempty"`
	TimeoutSeconds int   `json:"timeout,omitempty"`
}

type SendMessageRequest struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64     `json:"message_id"`
	From      *User     `json:"from,omitempty"`
	Chat      Chat      `json:"chat"`
	Text      string    `json:"text,omitempty"`
	Caption   string    `json:"caption,omitempty"`
	Photo     []Photo   `json:"photo,omitempty"`
	Document  *Document `json:"document,omitempty"`
	Voice     *Voice    `json:"voice,omitempty"`
	Audio     *Audio    `json:"audio,omitempty"`
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username,omitempty"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

type Photo struct {
	FileID string `json:"file_id"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type Audio struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      T      `json:"result"`
}

func NewHTTPClient(httpClient *http.Client, endpoint string, token string) *HTTPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	return &HTTPClient{
		client:   httpClient,
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    strings.TrimSpace(token),
	}
}

func (c *HTTPClient) GetUpdates(ctx context.Context, req GetUpdatesRequest) ([]Update, error) {
	var response apiResponse[[]Update]
	if err := c.post(ctx, "getUpdates", req, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (c *HTTPClient) SendMessage(ctx context.Context, req SendMessageRequest) error {
	var response apiResponse[json.RawMessage]
	return c.post(ctx, "sendMessage", req, &response)
}

func (c *HTTPClient) GetFile(ctx context.Context, fileID string) (File, error) {
	var response apiResponse[File]
	err := c.post(ctx, "getFile", map[string]string{"file_id": strings.TrimSpace(fileID)}, &response)
	return response.Result, err
}

func (c *HTTPClient) DownloadFile(ctx context.Context, filePath string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/file/bot"+c.token+"/"+strings.TrimLeft(filePath, "/"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		err = redactTelegramError(err)
		logger := logx.Component("telegram")
		logger.Error().Err(err).Str("event", "telegram.api.download.error").Str("file_path", strings.TrimSpace(filePath)).Msg("")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("telegram download failed: %s", strings.TrimSpace(string(body)))
		logger := logx.Component("telegram")
		logger.Error().Err(err).Str("event", "telegram.api.download.status").Int("status", resp.StatusCode).Msg("")
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		logger := logx.Component("telegram")
		logger.Error().Err(err).Str("event", "telegram.api.download.read").Msg("")
		return nil, err
	}
	logger := logx.Component("telegram")
	logger.Debug().Str("event", "telegram.api.download.ok").Int("bytes", len(data)).Msg("")
	return data, nil
}

func (c *HTTPClient) post(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("content-type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		err = redactTelegramError(err)
		logger := logx.Component("telegram")
		logger.Error().Err(err).Str("event", "telegram.api.error").Str("method", method).Msg("")
		return err
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		logger := logx.Component("telegram")
		logger.Error().Err(err).Str("event", "telegram.api.decode").Str("method", method).Msg("")
		return err
	}
	switch typed := out.(type) {
	case *apiResponse[[]Update]:
		if !typed.OK {
			err := fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(typed.Description))
			logger := logx.Component("telegram")
			logger.Error().Err(err).Str("event", "telegram.api.failed").Str("method", method).Msg("")
			return err
		}
	case *apiResponse[json.RawMessage]:
		if !typed.OK {
			err := fmt.Errorf("telegram %s failed: %s", method, strings.TrimSpace(typed.Description))
			logger := logx.Component("telegram")
			logger.Error().Err(err).Str("event", "telegram.api.failed").Str("method", method).Msg("")
			return err
		}
	}
	logger := logx.Component("telegram")
	logger.Debug().Str("event", "telegram.api.ok").Str("method", method).Msg("")
	return nil
}

func redactTelegramError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", telegramTokenPattern.ReplaceAllString(err.Error(), "${1}REDACTED/"))
}

var telegramTokenPattern = regexp.MustCompile(`(/(?:file/)?bot)[^/]+/`)
