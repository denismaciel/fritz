package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"fritz/internal/protocol/sse"
)

type RunSummary struct {
	RunID       string
	SessionPath string
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
