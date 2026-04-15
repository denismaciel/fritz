package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"fritz/internal/config"
	"fritz/internal/engine"
	"fritz/internal/logx"
	"fritz/internal/model"
)

type ChatType string

const (
	ChatTypeDM    ChatType = "dm"
	ChatTypeGroup ChatType = "group"
)

type InboundMessage struct {
	Channel    string
	ChatType   ChatType
	ChatID     string
	UserID     string
	SessionKey string
	Text       string
	Metadata   map[string]string
}

type OutboundMessage struct {
	Channel string
	ChatID  string
	UserID  string
	Text    string
}

type HandleResult struct {
	SessionKey string
	Session    engine.SessionRef
	Messages   []OutboundMessage
}

type Gateway struct {
	cwd     string
	cfg     config.Runtime
	engine  engine.Service
	router  Router
	paths   StatePaths
	state   SessionMapFile
	stateMu sync.Mutex
}

func New(cwd string, cfg config.Runtime, service engine.Service) *Gateway {
	return NewWithRouter(cwd, cfg, service, DefaultRouter{})
}

func NewWithRouter(cwd string, cfg config.Runtime, service engine.Service, router Router) *Gateway {
	if router == nil {
		router = DefaultRouter{}
	}
	return &Gateway{
		cwd:    cwd,
		cfg:    cfg,
		engine: service,
		router: router,
		paths:  ResolveStatePaths(cwd, cfg),
		state: SessionMapFile{
			Version:  CurrentStoreVersion,
			Sessions: map[string]string{},
		},
	}
}

func (g *Gateway) StateRoot() string {
	return g.paths.Root
}

func (g *Gateway) Paths() StatePaths {
	return g.paths
}

func (g *Gateway) HandleInbound(ctx context.Context, message InboundMessage) (HandleResult, error) {
	return g.HandleInboundStream(ctx, message, nil)
}

func (g *Gateway) HandleInboundStream(ctx context.Context, message InboundMessage, emit func(engine.Event) error) (HandleResult, error) {
	logger := logx.Component("gateway").With().
		Str("event", "gateway.inbound.handle").
		Str("channel", strings.TrimSpace(message.Channel)).
		Str("chat_type", string(message.ChatType)).
		Str("chat_id", strings.TrimSpace(message.ChatID)).
		Str("user_id", strings.TrimSpace(message.UserID)).
		Int("text_len", len(strings.TrimSpace(message.Text))).
		Logger()
	if strings.TrimSpace(message.Text) == "" {
		err := fmt.Errorf("empty inbound text")
		logger.Warn().Err(err).Msg("")
		return HandleResult{}, err
	}
	sessionKey, err := g.router.SessionKey(message)
	if err != nil {
		logger.Error().Err(err).Msg("")
		return HandleResult{}, err
	}
	logger = logger.With().Str("session_key", sessionKey).Logger()

	if err := EnsureLayout(g.paths, time.Now().UTC()); err != nil {
		logger.Error().Err(err).Str("event", "gateway.layout.ensure").Msg("")
		return HandleResult{}, err
	}
	state, err := g.loadState()
	if err != nil {
		logger.Error().Err(err).Str("event", "gateway.state.load").Msg("")
		return HandleResult{}, err
	}
	options := engine.SessionOptions{}
	if path := strings.TrimSpace(state.Sessions[sessionKey]); path != "" {
		options.SessionPath = path
	}
	logger.Info().Str("event", "gateway.session.resolved").Str("session_path", options.SessionPath).Msg("")

	session, err := g.engine.Start(ctx, options)
	if err != nil {
		logger.Error().Err(err).Str("event", "gateway.engine.start").Msg("")
		return HandleResult{}, err
	}
	run, err := session.Submit(ctx, engine.RunRequest{Prompt: strings.TrimSpace(message.Text)})
	if err != nil {
		logger.Error().Err(err).Str("event", "gateway.run.submit").Msg("")
		return HandleResult{}, err
	}

	replyText := ""
	for event := range run.Events {
		if emit != nil {
			if err := emit(event); err != nil {
				logger.Error().Err(err).Str("event", "gateway.event.emit").Msg("")
				return HandleResult{}, err
			}
		}
		if event.Kind != engine.EventMessageCompleted || event.Message == nil {
			continue
		}
		if event.Message.Role != model.ModelRole {
			continue
		}
		if text := strings.TrimSpace(event.Message.Text()); text != "" {
			replyText = text
		}
	}
	result := <-run.Done
	if result.Err != nil {
		logger.Error().Err(result.Err).Str("event", "gateway.run.done").Msg("")
		return HandleResult{}, result.Err
	}
	if replyText == "" {
		replyText = lastAssistantText(result.State.Messages)
	}
	if replyText == "" && len(result.State.Transcript) > 0 {
		replyText = strings.TrimSpace(result.State.Transcript[len(result.State.Transcript)-1].Assistant)
	}

	if result.Session.Path != "" && state.Sessions[sessionKey] != result.Session.Path {
		state.Sessions[sessionKey] = result.Session.Path
		if err := g.saveState(state); err != nil {
			logger.Error().Err(err).Str("event", "gateway.state.save").Str("session_path", result.Session.Path).Msg("")
			return HandleResult{}, err
		}
	}
	logger.Info().
		Str("event", "gateway.inbound.handled").
		Str("session_path", result.Session.Path).
		Int("reply_len", len(replyText)).
		Msg("")

	return HandleResult{
		SessionKey: sessionKey,
		Session:    result.Session,
		Messages: []OutboundMessage{{
			Channel: message.Channel,
			ChatID:  message.ChatID,
			UserID:  message.UserID,
			Text:    replyText,
		}},
		}, nil
}

func (g *Gateway) ClearSessionKey(_ context.Context, sessionKey string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return fmt.Errorf("missing session key")
	}
	state, err := g.loadState()
	if err != nil {
		return err
	}
	delete(state.Sessions, sessionKey)
	return g.saveState(state)
}

func (g *Gateway) loadState() (SessionMapFile, error) {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()

	state, exists, err := ReadJSONFile(g.paths.RoutingSessionMapPath, SessionMapFile{})
	if err != nil {
		return SessionMapFile{}, err
	}
	if !exists {
		state, exists, err = g.loadLegacyState()
		if err != nil {
			return SessionMapFile{}, err
		}
		if !exists {
			return g.copyStateLocked(), nil
		}
		if err := g.writeStateLocked(state); err != nil {
			return SessionMapFile{}, err
		}
	} else if state.Version == 0 {
		state.Version = CurrentStoreVersion
		if state.Sessions == nil {
			state.Sessions = map[string]string{}
		}
		if err := g.writeStateLocked(state); err != nil {
			return SessionMapFile{}, err
		}
	}
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	g.state = state
	return g.copyStateLocked(), nil
}

func (g *Gateway) saveState(state SessionMapFile) error {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()

	return g.writeStateLocked(state)
}

func (g *Gateway) writeStateLocked(state SessionMapFile) error {
	if state.Version == 0 {
		state.Version = CurrentStoreVersion
	}
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	g.state = state
	return WriteJSONFileAtomic(g.paths.RoutingSessionMapPath, g.state)
}

func (g *Gateway) copyStateLocked() SessionMapFile {
	out := SessionMapFile{Version: g.state.Version, Sessions: map[string]string{}}
	for key, path := range g.state.Sessions {
		out.Sessions[key] = path
	}
	return out
}

func (g *Gateway) loadLegacyState() (SessionMapFile, bool, error) {
	data, err := os.ReadFile(g.paths.legacyStatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionMapFile{}, false, nil
		}
		return SessionMapFile{}, false, err
	}
	var legacy struct {
		Sessions map[string]string `json:"sessions"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return SessionMapFile{}, true, err
	}
	return SessionMapFile{
		Version:  CurrentStoreVersion,
		Sessions: legacy.Sessions,
	}, true, nil
}

func lastAssistantText(messages []model.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != model.ModelRole {
			continue
		}
		if text := strings.TrimSpace(messages[i].Text()); text != "" {
			return text
		}
	}
	return ""
}
