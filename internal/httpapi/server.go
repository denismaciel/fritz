package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"fritz/internal/engine"
	"fritz/internal/protocol/agui"
	"fritz/internal/protocol/aisdk"
	"fritz/internal/protocol/sse"
)

type RunRequest struct {
	Prompt  string              `json:"prompt"`
	Session SessionStartOptions `json:"session,omitempty"`
}

type SessionStartOptions struct {
	Continue    bool   `json:"continue,omitempty"`
	SessionPath string `json:"sessionPath,omitempty"`
	ForkPath    string `json:"forkPath,omitempty"`
	NoSession   bool   `json:"noSession,omitempty"`
	NewSession  bool   `json:"newSession,omitempty"`
}

type Server struct {
	service engine.Service
}

func NewHandler(service engine.Service) http.Handler {
	return &Server{service: service}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/runs":
		s.handleRun(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/ai-sdk/chat":
		s.handleAISDKChat(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/runs/") && strings.HasSuffix(r.URL.Path, "/cancel"):
		s.handleCancel(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "missing prompt", http.StatusBadRequest)
		return
	}
	runtime, err := s.service.Start(r.Context(), toSessionOptions(req.Session))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	handle, err := runtime.Submit(r.Context(), engine.RunRequest{Prompt: req.Prompt})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	encoder := agui.NewEncoder()
	for event := range handle.Events {
		if err := encoder.WriteEvent(w, event); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	<-handle.Done
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/runs/"), "/cancel")
	runID = strings.Trim(runID, "/")
	ok := s.service.Cancel(runID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    ok,
		"runId": runID,
	})
}

type aiSDKChatRequest struct {
	Messages []aiSDKMessage      `json:"messages"`
	Session  SessionStartOptions `json:"session,omitempty"`
}

type aiSDKMessage struct {
	Role    string      `json:"role"`
	Content string      `json:"content,omitempty"`
	Parts   []aiSDKPart `json:"parts,omitempty"`
}

type aiSDKPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func (s *Server) handleAISDKChat(w http.ResponseWriter, r *http.Request) {
	var req aiSDKChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	prompt := extractAISDKPrompt(req.Messages)
	if strings.TrimSpace(prompt) == "" {
		http.Error(w, "missing user message", http.StatusBadRequest)
		return
	}
	runtime, err := s.service.Start(r.Context(), toSessionOptions(req.Session))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	handle, err := runtime.Submit(r.Context(), engine.RunRequest{Prompt: prompt})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("x-vercel-ai-ui-message-stream", "v1")
	flusher, _ := w.(http.Flusher)
	encoder := aisdk.NewEncoder()
	for event := range handle.Events {
		if err := encoder.WriteEvent(w, event); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	<-handle.Done
	_ = sse.WriteDone(w)
	if flusher != nil {
		flusher.Flush()
	}
}

func extractAISDKPrompt(messages []aiSDKMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
		var parts []string
		for _, part := range messages[i].Parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func toSessionOptions(options SessionStartOptions) engine.SessionOptions {
	return engine.SessionOptions{
		Continue:    options.Continue,
		SessionPath: options.SessionPath,
		ForkPath:    options.ForkPath,
		NoSession:   options.NoSession,
		NewSession:  options.NewSession,
	}
}

func StreamRun(ctx context.Context, client *http.Client, baseURL string, req RunRequest, emit func(sseEvent map[string]any) error) error {
	return streamRun(ctx, client, baseURL, req, emit)
}
