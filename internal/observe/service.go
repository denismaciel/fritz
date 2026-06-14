package observe

import (
	"context"

	"fritz/internal/chat"
	"fritz/internal/engine"
)

type Service struct {
	base engine.Service
	hub  *Hub
}

func WrapService(base engine.Service, hub *Hub) *Service {
	return &Service{base: base, hub: hub}
}

func (s *Service) Start(ctx context.Context, options engine.SessionOptions) (engine.Session, error) {
	session, err := s.base.Start(ctx, options)
	if err != nil {
		return nil, err
	}
	return &Session{base: session, hub: s.hub}, nil
}

func (s *Service) Cancel(runID string) bool {
	return s.base.Cancel(runID)
}

type Session struct {
	base engine.Session
	hub  *Hub
}

func (s *Session) Submit(ctx context.Context, req engine.RunRequest) (engine.Run, error) {
	run, err := s.base.Submit(ctx, req)
	if err != nil {
		return engine.Run{}, err
	}
	if s.hub == nil {
		return run, nil
	}
	s.hub.StartRun(run.ID, req.Prompt)
	events := make(chan engine.Event, 32)
	go func() {
		defer close(events)
		defer s.hub.FinishSubscribers(run.ID)
		for event := range run.Events {
			s.hub.Publish(event)
			events <- event
		}
	}()
	return engine.Run{
		ID:     run.ID,
		Events: events,
		Done:   run.Done,
	}, nil
}

func (s *Session) Reset() {
	s.base.Reset()
}

func (s *Session) State() chat.State {
	return s.base.State()
}

func (s *Session) CancelRun(runID string) bool {
	return s.base.CancelRun(runID)
}
