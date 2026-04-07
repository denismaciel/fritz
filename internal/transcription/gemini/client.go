package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"fritz/internal/transcription"
)

const (
	defaultEndpoint = "https://generativelanguage.googleapis.com"
	defaultModel    = "gemini-3-flash-preview"
)

type Client struct {
	apiKey     string
	endpoint   string
	model      string
	httpClient *http.Client
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		c.endpoint = strings.TrimRight(endpoint, "/")
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
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

func NewClient(apiKey string, options ...Option) *Client {
	client := &Client{
		apiKey:     strings.TrimSpace(apiKey),
		endpoint:   defaultEndpoint,
		model:      defaultModel,
		httpClient: http.DefaultClient,
	}
	for _, option := range options {
		option(client)
	}
	return client
}

func (c *Client) Transcribe(ctx context.Context, input transcription.AudioInput) (string, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return "", errors.New("gemini api key missing")
	}
	if len(input.Bytes) == 0 {
		return "", errors.New("audio bytes missing")
	}
	mimeType := strings.TrimSpace(input.MIMEType)
	if mimeType == "" {
		mimeType = "audio/ogg"
	}
	uploadURL, err := c.startUpload(ctx, mimeType, len(input.Bytes), input.FileName)
	if err != nil {
		return "", err
	}
	fileURI, fileMIME, err := c.uploadBytes(ctx, uploadURL, input.Bytes)
	if err != nil {
		return "", err
	}
	if fileMIME == "" {
		fileMIME = mimeType
	}
	return c.generateTranscript(ctx, fileURI, fileMIME)
}

func (c *Client) startUpload(ctx context.Context, mimeType string, size int, fileName string) (string, error) {
	payload := map[string]any{
		"file": map[string]string{
			"display_name": displayName(fileName),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.endpoint, "/")+"/upload/v1beta/files", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("x-goog-upload-protocol", "resumable")
	req.Header.Set("x-goog-upload-command", "start")
	req.Header.Set("x-goog-upload-header-content-length", fmt.Sprintf("%d", size))
	req.Header.Set("x-goog-upload-header-content-type", mimeType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini upload start failed: %s", strings.TrimSpace(string(body)))
	}
	uploadURL := strings.TrimSpace(resp.Header.Get("x-goog-upload-url"))
	if uploadURL == "" {
		return "", errors.New("gemini upload url missing")
	}
	return uploadURL, nil
}

func (c *Client) uploadBytes(ctx context.Context, uploadURL string, body []byte) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("content-length", fmt.Sprintf("%d", len(body)))
	req.Header.Set("x-goog-upload-offset", "0")
	req.Header.Set("x-goog-upload-command", "upload, finalize")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("gemini upload finalize failed: %s", strings.TrimSpace(string(data)))
	}
	var payload struct {
		File struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
		} `json:"file"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(payload.File.URI) == "" {
		return "", "", errors.New("gemini uploaded file uri missing")
	}
	return payload.File.URI, payload.File.MIMEType, nil
}

func (c *Client) generateTranscript(ctx context.Context, fileURI string, mimeType string) (string, error) {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"text": "Transcribe this audio. Return only the spoken transcript as plain text. Do not add commentary or formatting.",
					},
					{
						"file_data": map[string]string{
							"mime_type": mimeType,
							"file_uri":  fileURI,
						},
					},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(c.endpoint, "/"), c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini transcription failed: %s", strings.TrimSpace(string(data)))
	}
	var payloadResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return "", err
	}
	var out strings.Builder
	for _, candidate := range payloadResp.Candidates {
		for _, part := range candidate.Content.Parts {
			out.WriteString(part.Text)
		}
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", errors.New("gemini transcription empty")
	}
	return text, nil
}

func displayName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "audio"
	}
	return fileName
}
