package engine

import (
	"context"

	"fritz/internal/agent"
	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/model"
	"fritz/internal/session"
	"fritz/internal/tool"
)

type Event = agent.Event
type EventKind = agent.EventKind
type SessionRef = agent.SessionRef
type RunResult = agent.RunResult

const (
	EventRunStarted         = agent.EventRunStarted
	EventStepStarted        = agent.EventStepStarted
	EventReasoningStarted   = agent.EventReasoningStarted
	EventReasoningDelta     = agent.EventReasoningDelta
	EventReasoningCompleted = agent.EventReasoningCompleted
	EventTextDelta          = agent.EventTextDelta
	EventMessageCompleted   = agent.EventMessageCompleted
	EventToolCallStarted    = agent.EventToolCallStarted
	EventToolCallCompleted  = agent.EventToolCallCompleted
	EventStepFinished       = agent.EventStepFinished
	EventRunFinished        = agent.EventRunFinished
	EventRunFailed          = agent.EventRunFailed
	EventRunCanceled        = agent.EventRunCanceled
)

type RunRequest struct {
	Prompt string
}

type SessionOptions struct {
	Continue    bool
	SessionPath string
	ForkPath    string
	NoSession   bool
	NewSession  bool
}

type Run struct {
	ID     string
	Events <-chan Event
	Done   <-chan RunResult
}

type Session interface {
	Submit(context.Context, RunRequest) (Run, error)
	Reset()
	State() chat.State
	CancelRun(runID string) bool
}

type Service interface {
	Start(context.Context, SessionOptions) (Session, error)
	Cancel(runID string) bool
}

type ClientFactory = agent.ClientFactory
type RegistryFactory = agent.RegistryFactory

type LocalService struct {
	base *agent.Service
}

func NewLocalService(
	cwd string,
	cfg config.Runtime,
	newClient ClientFactory,
	newRegistry RegistryFactory,
) *LocalService {
	return &LocalService{
		base: agent.NewService(cwd, cfg, newClient, newRegistry),
	}
}

func WrapService(base *agent.Service) *LocalService {
	return &LocalService{base: base}
}

func (s *LocalService) Start(ctx context.Context, options SessionOptions) (Session, error) {
	runtime, err := s.base.NewRuntime(ctx, agent.RuntimeOptions{
		Session: session.StartOptions{
			Continue:    options.Continue,
			SessionPath: options.SessionPath,
			ForkPath:    options.ForkPath,
			NoSession:   options.NoSession,
			NewSession:  options.NewSession,
		},
	})
	if err != nil {
		return nil, err
	}
	return localSession{base: runtime}, nil
}

func (s *LocalService) Cancel(runID string) bool {
	return s.base.Cancel(runID)
}

type localSession struct {
	base *agent.Runtime
}

func (s localSession) Submit(ctx context.Context, req RunRequest) (Run, error) {
	handle, err := s.base.Submit(ctx, agent.RunRequest{Prompt: req.Prompt})
	if err != nil {
		return Run{}, err
	}
	return Run{
		ID:     handle.ID,
		Events: handle.Events,
		Done:   handle.Done,
	}, nil
}

func (s localSession) Reset() {
	s.base.Reset()
}

func (s localSession) State() chat.State {
	return s.base.State()
}

func (s localSession) CancelRun(runID string) bool {
	return s.base.CancelRun(runID)
}

func NewRegistryFactory(registry *tool.Registry) RegistryFactory {
	return func(config.Runtime) *tool.Registry {
		if registry == nil {
			return tool.NewRegistry()
		}
		return registry
	}
}

func NewClientFactory(factory func(config.Runtime) model.Client) ClientFactory {
	return agent.ClientFactory(factory)
}
