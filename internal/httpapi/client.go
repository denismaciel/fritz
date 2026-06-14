package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"fritz/internal/observe"
	"fritz/internal/protocol/sse"
)

type RunSummary struct {
	RunID       string
	SessionPath string
}

type RunsResponse struct {
	Runs []observe.RunInfo `json:"runs"`
}

func NewUnixSocketClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

func streamRun(ctx context.Context, client *http.Client, baseURL string, req RunRequest, emit func(sseEvent map[string]any) error) error {
	if client == nil {
		client = http.DefaultClient
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/runs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s", strings.TrimSpace(string(data)))
	}
	return sse.Read(resp.Body, func(event sse.Event) error {
		if event.Data == "[DONE]" {
			return nil
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			return err
		}
		return emit(payload)
	})
}

func StreamRunPayloads(ctx context.Context, client *http.Client, baseURL string, req RunRequest, emit func(map[string]any) error) error {
	return streamRun(ctx, client, baseURL, req, emit)
}

func GetGatewaySession(ctx context.Context, client *http.Client, baseURL string, key string) (GatewaySessionResponse, error) {
	if client == nil {
		client = http.DefaultClient
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/gateway/session?key="+url.QueryEscape(key), nil)
	if err != nil {
		return GatewaySessionResponse{}, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return GatewaySessionResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return GatewaySessionResponse{}, fmt.Errorf("server error: %s", strings.TrimSpace(string(data)))
	}
	var out GatewaySessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return GatewaySessionResponse{}, err
	}
	return out, nil
}

func ListRuns(ctx context.Context, client *http.Client, baseURL string) ([]observe.RunInfo, error) {
	if client == nil {
		client = http.DefaultClient
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/runs", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s", strings.TrimSpace(string(data)))
	}
	var out RunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Runs, nil
}

func AttachRun(ctx context.Context, client *http.Client, baseURL string, runID string, out io.Writer) error {
	if client == nil {
		client = http.DefaultClient
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("missing run id")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/runs/"+runID+"/events", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s", strings.TrimSpace(string(data)))
	}
	return renderAGUIStream(resp.Body, out)
}

func RenderAGUIRun(ctx context.Context, client *http.Client, baseURL string, req RunRequest, out io.Writer) (RunSummary, error) {
	var summary RunSummary
	err := streamRun(ctx, client, baseURL, req, func(payload map[string]any) error {
		switch payload["type"] {
		case "RUN_STARTED":
			summary.RunID, _ = payload["runId"].(string)
			summary.SessionPath, _ = payload["sessionPath"].(string)
		case "TEXT_MESSAGE_CONTENT":
			if delta, ok := payload["delta"].(string); ok {
				if _, err := io.WriteString(out, delta); err != nil {
					return err
				}
			}
		case "TEXT_MESSAGE_END":
			if _, err := io.WriteString(out, "\n"); err != nil {
				return err
			}
		case "TOOL_CALL_START":
			name, _ := payload["toolName"].(string)
			if _, err := fmt.Fprintf(out, "[tool] %s\n", name); err != nil {
				return err
			}
		case "RUN_ERROR":
			message, _ := payload["message"].(string)
			return errors.New(message)
		}
		return nil
	})
	return summary, err
}

func renderAGUIStream(in io.Reader, out io.Writer) error {
	return sse.Read(in, func(event sse.Event) error {
		if event.Data == "[DONE]" {
			return nil
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			return err
		}
		return renderAGUIPayload(payload, out)
	})
}

func renderAGUIPayload(payload map[string]any, out io.Writer) error {
	switch payload["type"] {
	case "TEXT_MESSAGE_CONTENT":
		if delta, ok := payload["delta"].(string); ok {
			_, err := io.WriteString(out, delta)
			return err
		}
	case "TEXT_MESSAGE_END":
		_, err := io.WriteString(out, "\n")
		return err
	case "REASONING_MESSAGE_CONTENT":
		if delta, ok := payload["delta"].(string); ok {
			_, err := fmt.Fprintf(out, "[thinking] %s\n", delta)
			return err
		}
	case "TOOL_CALL_START":
		name, _ := payload["toolName"].(string)
		_, err := fmt.Fprintf(out, "[tool] %s\n", name)
		return err
	case "TOOL_CALL_ARGS":
		if args, ok := payload["args"]; ok && args != nil {
			_, err := fmt.Fprintf(out, "[args] %v\n", args)
			return err
		}
	case "RUN_ERROR":
		message, _ := payload["message"].(string)
		return errors.New(message)
	}
	return nil
}

func CancelRun(ctx context.Context, client *http.Client, baseURL string, runID string) error {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/runs/"+runID+"/cancel", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s", strings.TrimSpace(string(data)))
	}
	return nil
}
