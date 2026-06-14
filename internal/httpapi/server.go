package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"fritz/internal/chat"
	"fritz/internal/engine"
	"fritz/internal/observe"
	"fritz/internal/protocol/agui"
	"fritz/internal/protocol/aisdk"
	"fritz/internal/protocol/sse"
	"fritz/internal/session"
)

type RunRequest struct {
	Prompt         string              `json:"prompt"`
	Session        SessionStartOptions `json:"session,omitempty"`
	GatewaySession string              `json:"gatewaySession,omitempty"`
}

type SessionStartOptions struct {
	Continue    bool   `json:"continue,omitempty"`
	SessionPath string `json:"sessionPath,omitempty"`
	ForkPath    string `json:"forkPath,omitempty"`
	NoSession   bool   `json:"noSession,omitempty"`
	NewSession  bool   `json:"newSession,omitempty"`
}

type Server struct {
	service         engine.Service
	hub             *observe.Hub
	sessionResolver SessionResolver
}

type GatewaySessionResponse struct {
	Key        string          `json:"key"`
	Path       string          `json:"path"`
	Transcript chat.Transcript `json:"transcript"`
}

type SessionResolver interface {
	SessionPathForKey(context.Context, string) (string, error)
}

func NewHandler(service engine.Service) http.Handler {
	return &Server{service: service}
}

func NewHandlerWithHub(service engine.Service, hub *observe.Hub) http.Handler {
	return &Server{service: service, hub: hub}
}

func NewHandlerWithOptions(service engine.Service, hub *observe.Hub, resolver SessionResolver) http.Handler {
	return &Server{service: service, hub: hub, sessionResolver: resolver}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/runs":
		s.handleRun(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/runs":
		s.handleListRuns(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/runs/") && strings.HasSuffix(r.URL.Path, "/events"):
		s.handleRunEvents(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/gateway/session":
		s.handleGatewaySession(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/ai-sdk/chat":
		s.handleAISDKChat(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/runs/") && strings.HasSuffix(r.URL.Path, "/cancel"):
		s.handleCancel(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleGatewaySession(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	if s.sessionResolver == nil {
		http.Error(w, "gateway session resolver not configured", http.StatusNotFound)
		return
	}
	path, err := s.sessionResolver.SessionPathForKey(r.Context(), key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(path) == "" {
		http.Error(w, fmt.Sprintf("gateway session %q is not bound", key), http.StatusNotFound)
		return
	}
	manager, err := session.Open(path, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(GatewaySessionResponse{
		Key:        key,
		Path:       path,
		Transcript: manager.BuildContext().Transcript,
	})
}

func (s *Server) handleListRuns(w http.ResponseWriter, _ *http.Request) {
	if s.hub == nil {
		http.Error(w, "observe hub not configured", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"runs": s.hub.ListRuns(),
	})
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		http.Error(w, "observe hub not configured", http.StatusNotFound)
		return
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/runs/"), "/events")
	runID = strings.Trim(runID, "/")
	events, unsubscribe, ok := s.hub.Subscribe(runID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	encoder := agui.NewEncoder()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := encoder.WriteEvent(w, event); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
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
	options, err := s.resolveSessionOptions(r.Context(), req.Session, req.GatewaySession)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtime, err := s.service.Start(r.Context(), options)
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
	options, err := s.resolveSessionOptions(r.Context(), req.Session, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtime, err := s.service.Start(r.Context(), options)
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

func (s *Server) resolveSessionOptions(ctx context.Context, options SessionStartOptions, gatewaySession string) (engine.SessionOptions, error) {
	out := toSessionOptions(options)
	gatewaySession = strings.TrimSpace(gatewaySession)
	if gatewaySession == "" {
		return out, nil
	}
	if s.sessionResolver == nil {
		return engine.SessionOptions{}, fmt.Errorf("gateway session resolver not configured")
	}
	path, err := s.sessionResolver.SessionPathForKey(ctx, gatewaySession)
	if err != nil {
		return engine.SessionOptions{}, err
	}
	if strings.TrimSpace(path) == "" {
		return engine.SessionOptions{}, fmt.Errorf("gateway session %q is not bound", gatewaySession)
	}
	out.SessionPath = path
	out.Continue = false
	out.NewSession = false
	out.NoSession = false
	return out, nil
}

func StreamRun(ctx context.Context, client *http.Client, baseURL string, req RunRequest, emit func(sseEvent map[string]any) error) error {
	return streamRun(ctx, client, baseURL, req, emit)
}
