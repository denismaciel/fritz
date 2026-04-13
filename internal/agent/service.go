package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/expansion"
	"fritz/internal/logx"
	"fritz/internal/model"
	"fritz/internal/prompt"
	"fritz/internal/session"
	"fritz/internal/skill"
	"fritz/internal/tool"
)

type EventKind string

const (
	EventRunStarted         EventKind = "run_started"
	EventStepStarted        EventKind = "step_started"
	EventReasoningStarted   EventKind = "reasoning_started"
	EventReasoningDelta     EventKind = "reasoning_delta"
	EventReasoningCompleted EventKind = "reasoning_completed"
	EventTextDelta          EventKind = "text_delta"
	EventMessageCompleted   EventKind = "message_completed"
	EventToolCallStarted    EventKind = "tool_call_started"
	EventToolCallCompleted  EventKind = "tool_call_completed"
	EventStepFinished       EventKind = "step_finished"
	EventRunFinished        EventKind = "run_finished"
	EventRunFailed          EventKind = "run_failed"
	EventRunCanceled        EventKind = "run_canceled"
)

type SessionRef struct {
	ID   string `json:"id,omitempty"`
	Path string `json:"path,omitempty"`
}

type Event struct {
	ID         string         `json:"id"`
	RunID      string         `json:"runId"`
	MessageID  string         `json:"messageId,omitempty"`
	Kind       EventKind      `json:"kind"`
	Step       int            `json:"step,omitempty"`
	TextDelta  string         `json:"textDelta,omitempty"`
	Message    *model.Message `json:"message,omitempty"`
	ToolCall   *tool.Call     `json:"toolCall,omitempty"`
	ToolResult *tool.Result   `json:"toolResult,omitempty"`
	Error      string         `json:"error,omitempty"`
	Session    SessionRef     `json:"session,omitempty"`
	Time       time.Time      `json:"time"`
}

type RunRequest struct {
	Prompt string
	Images []tool.ContentPart
}

type RunResult struct {
	State   chat.State
	Session SessionRef
	Err     error
}

type RunHandle struct {
	ID     string
	Events <-chan Event
	Done   <-chan RunResult
}

type RuntimeOptions struct {
	Session session.StartOptions
}

type ClientFactory func(cfg config.Runtime) model.Client
type RegistryFactory func(cfg config.Runtime) *tool.Registry

type Service struct {
	cwd         string
	cfg         config.Runtime
	newClient   ClientFactory
	newRegistry RegistryFactory
	profile     prompt.Profile

	runSeq atomic.Uint64

	mu   sync.Mutex
	runs map[string]context.CancelFunc
}

func NewService(cwd string, cfg config.Runtime, newClient ClientFactory, newRegistry RegistryFactory) *Service {
	return NewServiceWithPromptProfile(cwd, cfg, newClient, newRegistry, prompt.ProfileCoding)
}

func NewServiceWithPromptProfile(cwd string, cfg config.Runtime, newClient ClientFactory, newRegistry RegistryFactory, profile prompt.Profile) *Service {
	if newRegistry == nil {
		newRegistry = func(config.Runtime) *tool.Registry {
			return tool.NewRegistry()
		}
	}
	return &Service{
		cwd:         cwd,
		cfg:         cfg,
		newClient:   newClient,
		newRegistry: newRegistry,
		profile:     profile,
		runs:        map[string]context.CancelFunc{},
	}
}

type Runtime struct {
	service       *Service
	cwd           string
	cfg           config.Runtime
	promptRuntime prompt.Runtime
	gateway       model.Client
	registry      *tool.Registry
	manager       *session.Manager

	mu    sync.Mutex
	state chat.State
}

func (s *Service) NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	promptRuntime, err := prompt.LoadRuntimeForProfile(s.cwd, s.cfg, s.profile)
	if err != nil {
		return nil, err
	}
	sessionRuntime, err := session.Start(ctx, s.cwd, s.cfg, options.Session)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		service:       s,
		cwd:           s.cwd,
		cfg:           s.cfg,
		promptRuntime: promptRuntime,
		registry:      s.newRegistry(s.cfg),
		manager:       sessionRuntime.Manager,
		state:         sessionRuntime.State,
	}, nil
}

func (s *Service) Cancel(runID string) bool {
	s.mu.Lock()
	cancel, ok := s.runs[runID]
	s.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

func (r *Runtime) State() chat.State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *Runtime) ModelID() string {
	return r.cfg.ModelID
}

func (r *Runtime) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = chat.NewState()
}

func (r *Runtime) CancelRun(runID string) bool {
	return r.service.Cancel(runID)
}

func (r *Runtime) Submit(ctx context.Context, req RunRequest) (RunHandle, error) {
	expanded, err := expandUserPrompt(req.Prompt, r.cwd, r.promptRuntime.Skills)
	if err != nil {
		return RunHandle{}, err
	}
	if r.gateway == nil {
		r.gateway = r.service.newClient(r.cfg)
	}
	if r.manager != nil {
		if _, err := r.manager.AppendPrompt(expanded); err != nil {
			return RunHandle{}, err
		}
	}

	r.mu.Lock()
	initial := chat.SubmitPromptWithImages(r.state, expanded, req.Images)
	r.state = initial.State
	r.mu.Unlock()

	runID := r.service.newRunID()
	messageID := runID + "-assistant"
	events := make(chan Event, 32)
	done := make(chan RunResult, 1)
	runCtx, cancel := context.WithCancel(ctx)
	r.service.registerRun(runID, cancel)

	go r.execute(runCtx, runID, messageID, initial, events, done)

	return RunHandle{
		ID:     runID,
		Events: events,
		Done:   done,
	}, nil
}

func (r *Runtime) execute(ctx context.Context, runID string, messageID string, result chat.Result, events chan<- Event, done chan<- RunResult) {
	defer close(events)
	defer close(done)
	defer r.service.unregisterRun(runID)

	emitter := newEmitter(runID, messageID, events)
	emitter.emit(Event{Kind: EventRunStarted, Session: sessionRef(r.manager)})
	logger := logx.Component("agent").With().
		Str("event", "agent.run").
		Str("run_id", runID).
		Str("message_id", messageID).
		Str("session_id", sessionRef(r.manager).ID).
		Str("session_path", sessionRef(r.manager).Path).
		Logger()
	ctx = logx.WithContext(ctx, logger)
	logger.Info().Str("stage", "start").Int("queued_effects", len(result.Effects)).Msg("")

	state := result.State
	queue := append([]chat.Effect{}, result.Effects...)
	step := 0

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			logger.Warn().Err(ctx.Err()).Str("stage", "canceled").Msg("")
			emitter.emit(Event{Kind: EventRunCanceled, Error: ctx.Err().Error(), Session: sessionRef(r.manager)})
			done <- RunResult{State: state, Session: sessionRef(r.manager), Err: ctx.Err()}
			return
		default:
		}

		effect := queue[0]
		queue = queue[1:]

		switch effect := effect.(type) {
		case chat.Print:
			emitter.emit(Event{
				Kind:      EventMessageCompleted,
				Message:   messagePtr(model.TextMessage(model.ModelRole, effect.Line)),
				Session:   sessionRef(r.manager),
				MessageID: messageID,
			})
		case chat.CallModel:
			step++
			emitter.emit(Event{Kind: EventStepStarted, Step: step, Session: sessionRef(r.manager)})
			req := model.Request{
				SystemPrompt: buildSystemPrompt(r.cwd, r.promptRuntime, r.registry),
				Messages:     effect.Messages,
				Tools:        r.registry.Definitions(),
				ModelID:      r.cfg.ModelID,
			}
			if r.manager != nil {
				compactedReq, compacted, err := session.MaybeCompactRequest(ctx, r.manager, r.cfg.Session, r.gateway, req)
				if err != nil {
					logger.Error().Err(err).Str("stage", "session.pre_model_compact").Int("step", step).Msg("")
					emitter.emit(Event{Kind: EventRunFailed, Error: err.Error(), Session: sessionRef(r.manager)})
					done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
					return
				}
				if compacted {
					req = compactedReq
					context := r.manager.BuildContext()
					state.Transcript = context.Transcript
					state.Messages = context.Messages
				}
			}
			logger.Info().Str("stage", "model.start").Int("step", step).Int("messages", len(req.Messages)).Int("tools", len(req.Tools)).Msg("")
			response, err := r.callModel(ctx, emitter, step, req)
			if err != nil {
				if r.manager != nil && session.ShouldRetryAfterContextOverflow(err) {
					response, _, err = session.RetryAfterOverflow(ctx, r.manager, r.cfg.Session, r.gateway, req, func(req model.Request) (model.Response, error) {
						return r.gateway.Generate(ctx, req)
					})
				}
			}
			if err != nil {
				if errors.Is(err, context.Canceled) {
					logger.Warn().Err(err).Str("stage", "model.canceled").Int("step", step).Msg("")
					emitter.emit(Event{Kind: EventRunCanceled, Error: err.Error(), Session: sessionRef(r.manager)})
					done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
					return
				}
				logger.Error().Err(err).Str("stage", "model.error").Int("step", step).Msg("")
				emitter.emit(Event{Kind: EventRunFailed, Error: err.Error(), Session: sessionRef(r.manager)})
				done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
				return
			}
			logger.Info().Str("stage", "model.finish").Int("step", step).Int("tool_calls", len(response.ToolCalls)).Int("text_len", len(response.Message.Text())).Msg("")
			if r.manager != nil {
				if _, err := r.manager.AppendModelResponse(response); err != nil {
					logger.Error().Err(err).Str("stage", "session.append_model").Int("step", step).Msg("")
					emitter.emit(Event{Kind: EventRunFailed, Error: err.Error(), Session: sessionRef(r.manager)})
					done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
					return
				}
			}
			state = chat.ApplyModelResponse(state, response)
			if len(response.ToolCalls) == 0 {
				emitter.emit(Event{
					Kind:      EventMessageCompleted,
					MessageID: messageID,
					Message:   messagePtr(response.Message),
					Session:   sessionRef(r.manager),
					Step:      step,
				})
				emitter.emit(Event{Kind: EventStepFinished, Step: step, Session: sessionRef(r.manager)})
				if r.manager != nil {
					compacted, err := session.MaybeAutoCompact(ctx, r.manager, r.cfg.Session, r.gateway, r.cfg.ModelID)
					if err != nil {
						logger.Error().Err(err).Str("stage", "session.compact").Int("step", step).Msg("")
						emitter.emit(Event{Kind: EventRunFailed, Error: err.Error(), Session: sessionRef(r.manager)})
						done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
						return
					}
					if compacted {
						logger.Info().Str("stage", "session.compacted").Int("step", step).Msg("")
						context := r.manager.BuildContext()
						state.Transcript = context.Transcript
						state.Messages = context.Messages
					}
				}
				continue
			}
			for _, call := range response.ToolCalls {
				emitter.emit(Event{Kind: EventToolCallStarted, Step: step, ToolCall: callPtr(call), Session: sessionRef(r.manager)})
				toolResult, err := r.registry.Run(r.toolContext(ctx), call)
				if err != nil && toolResult.Text() == "" {
					toolResult = tool.ErrorTextResult(call, err)
				}
				emitter.emit(Event{Kind: EventToolCallCompleted, Step: step, ToolCall: callPtr(call), ToolResult: resultPtr(toolResult), Session: sessionRef(r.manager)})
				if r.manager != nil {
					message := model.Message{
						Role: model.UserRole,
						Parts: []model.Part{
							{ToolResult: &toolResult},
						},
					}
					if _, err := r.manager.AppendToolResult(toolResult.Text(), message); err != nil {
						logger.Error().Err(err).Str("stage", "session.append_tool").Int("step", step).Str("tool", call.Name).Msg("")
						emitter.emit(Event{Kind: EventRunFailed, Error: err.Error(), Session: sessionRef(r.manager)})
						done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
						return
					}
				}
				state.Messages = append(state.Messages, model.Message{
					Role: model.UserRole,
					Parts: []model.Part{
						{ToolResult: &toolResult},
					},
				})
			}
			emitter.emit(Event{Kind: EventStepFinished, Step: step, Session: sessionRef(r.manager)})
			queue = append([]chat.Effect{
				chat.CallModel{Messages: append([]model.Message(nil), state.Messages...)},
			}, queue...)
		case chat.RunTool:
			emitter.emit(Event{Kind: EventToolCallStarted, Step: step, ToolCall: callPtr(effect.Call), Session: sessionRef(r.manager)})
			toolResult, err := r.registry.Run(r.toolContext(ctx), effect.Call)
			if err != nil && toolResult.Text() == "" {
				toolResult = tool.ErrorTextResult(effect.Call, err)
			}
			emitter.emit(Event{Kind: EventToolCallCompleted, Step: step, ToolCall: callPtr(effect.Call), ToolResult: resultPtr(toolResult), Session: sessionRef(r.manager)})
			if r.manager != nil {
				message := model.Message{
					Role: model.UserRole,
					Parts: []model.Part{
						{ToolResult: &toolResult},
					},
				}
				if _, err := r.manager.AppendToolResult(toolResult.Text(), message); err != nil {
					logger.Error().Err(err).Str("stage", "session.append_tool").Int("step", step).Str("tool", effect.Call.Name).Msg("")
					emitter.emit(Event{Kind: EventRunFailed, Error: err.Error(), Session: sessionRef(r.manager)})
					done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
					return
				}
			}
			next := chat.HandleToolResult(state, toolResult)
			state = next.State
			queue = append(next.Effects, queue...)
		default:
			err := fmt.Errorf("unsupported effect %T", effect)
			logger.Error().Err(err).Str("stage", "effect.unsupported").Msg("")
			emitter.emit(Event{Kind: EventRunFailed, Error: err.Error(), Session: sessionRef(r.manager)})
			done <- RunResult{State: state, Session: sessionRef(r.manager), Err: err}
			return
		}
	}

	r.mu.Lock()
	r.state = state
	r.mu.Unlock()
	logger.Info().Str("stage", "finish").Int("messages", len(state.Messages)).Int("transcript_turns", len(state.Transcript)).Msg("")
	emitter.emit(Event{Kind: EventRunFinished, Session: sessionRef(r.manager)})
	done <- RunResult{State: state, Session: sessionRef(r.manager)}
}

func (r *Runtime) callModel(ctx context.Context, emitter *eventEmitter, step int, req model.Request) (model.Response, error) {
	reasoningStarted := false
	reasoningID := fmt.Sprintf("%s-reasoning-%d", emitter.messageID, step)
	response, err := r.gateway.StreamGenerate(ctx, req, func(event model.StreamEvent) error {
		if event.ReasoningDelta != "" {
			if !reasoningStarted {
				reasoningStarted = true
				emitter.emit(Event{Kind: EventReasoningStarted, Step: step, MessageID: reasoningID, Session: sessionRef(r.manager)})
			}
			emitter.emit(Event{Kind: EventReasoningDelta, Step: step, MessageID: reasoningID, TextDelta: event.ReasoningDelta, Session: sessionRef(r.manager)})
		}
		if event.TextDelta != "" {
			emitter.emit(Event{Kind: EventTextDelta, Step: step, TextDelta: event.TextDelta, Session: sessionRef(r.manager)})
		}
		return nil
	})
	if err == nil {
		if reasoningStarted {
			emitter.emit(Event{Kind: EventReasoningCompleted, Step: step, MessageID: reasoningID, Session: sessionRef(r.manager)})
		} else if response.Message.ReasoningText() != "" {
			emitter.emit(Event{Kind: EventReasoningStarted, Step: step, MessageID: reasoningID, Session: sessionRef(r.manager)})
			emitter.emit(Event{Kind: EventReasoningDelta, Step: step, MessageID: reasoningID, TextDelta: response.Message.ReasoningText(), Session: sessionRef(r.manager)})
			emitter.emit(Event{Kind: EventReasoningCompleted, Step: step, MessageID: reasoningID, Session: sessionRef(r.manager)})
		}
		return response, nil
	}
	if errors.Is(err, context.Canceled) {
		return model.Response{}, err
	}
	return r.gateway.Generate(ctx, req)
}

func (r *Runtime) toolContext(ctx context.Context) context.Context {
	sessionPath := ""
	if r.manager != nil {
		sessionPath = r.manager.SessionFile()
	}
	logger := logx.FromContext(ctx)
	if sessionPath != "" {
		logger = logger.With().Str("session_path", sessionPath).Logger()
	}
	ctx = logx.WithContext(ctx, logger)
	return tool.WithRunContext(ctx, tool.RunContext{SessionPath: sessionPath})
}

type eventEmitter struct {
	runID     string
	messageID string
	events    chan<- Event
	seq       int
}

func newEmitter(runID string, messageID string, events chan<- Event) *eventEmitter {
	return &eventEmitter{runID: runID, messageID: messageID, events: events}
}

func (e *eventEmitter) emit(event Event) {
	e.seq++
	event.ID = fmt.Sprintf("%s-%03d", e.runID, e.seq)
	if event.RunID == "" {
		event.RunID = e.runID
	}
	if event.MessageID == "" {
		event.MessageID = e.messageID
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	e.events <- event
}

func (s *Service) newRunID() string {
	return fmt.Sprintf("run-%06d", s.runSeq.Add(1))
}

func (s *Service) registerRun(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[runID] = cancel
}

func (s *Service) unregisterRun(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, runID)
}

func sessionRef(manager *session.Manager) SessionRef {
	if manager == nil {
		return SessionRef{}
	}
	info := manager.Stats()
	return SessionRef{
		ID:   info.ID,
		Path: info.Path,
	}
}

func buildSystemPrompt(cwd string, promptRuntime prompt.Runtime, registry *tool.Registry) string {
	return prompt.BuildSystemPrompt(prompt.BuildOptions{
		Profile:            promptRuntime.Profile,
		Cwd:                cwd,
		Now:                time.Now().UTC(),
		Base:               promptRuntime.Resources.SystemPrompt,
		AppendSystemPrompt: promptRuntime.Resources.AppendSystemPrompt,
		ContextFiles:       promptRuntime.Resources.ContextFiles,
		MemoryFiles:        promptRuntime.Resources.MemoryFiles,
		HeartbeatFiles:     promptRuntime.Resources.HeartbeatFiles,
		Skills:             promptRuntime.Skills,
		Tools:              prompt.ToolPromptsFromDefinitions(registry.Definitions()),
	})
}

func expandUserPrompt(text string, cwd string, skills []skill.Skill) (string, error) {
	text, err := expansion.ExpandFiles(text, cwd)
	if err != nil {
		return "", err
	}
	expanded, _, err := skill.ExpandCommand(text, skills)
	if err != nil {
		return "", err
	}
	if expanded != "" {
		return expanded, nil
	}
	return text, nil
}

func messagePtr(message model.Message) *model.Message {
	return &message
}

func callPtr(call tool.Call) *tool.Call {
	return &call
}

func resultPtr(result tool.Result) *tool.Result {
	return &result
}
