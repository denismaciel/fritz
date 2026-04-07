package heartbeat

import (
	"context"
	"strings"

	"fritz/internal/engine"
	"fritz/internal/logx"
)

type SessionRunner struct {
	Service       engine.Service
	SessionPaths  func(context.Context, string) (string, error)
	Prompt        string
	PromptForWake func(Wake) string
}

func (r SessionRunner) Run(ctx context.Context, wake Wake) (Result, error) {
	logger := logx.Component("heartbeat").With().Str("event", "heartbeat.engine.run").Str("target_key", wake.TargetKey).Logger()
	options := engine.SessionOptions{}
	if r.SessionPaths != nil {
		path, err := r.SessionPaths(ctx, wake.TargetKey)
		if err != nil {
			logger.Error().Err(err).Str("stage", "session_path").Msg("")
			return Result{}, err
		}
		options.SessionPath = strings.TrimSpace(path)
	}
	session, err := r.Service.Start(ctx, options)
	if err != nil {
		logger.Error().Err(err).Str("stage", "engine.start").Msg("")
		return Result{}, err
	}
	prompt := r.Prompt
	if r.PromptForWake != nil {
		prompt = r.PromptForWake(wake)
	} else if strings.TrimSpace(wake.Reason) != "" {
		prompt = strings.TrimSpace(prompt) + "\n\nDue work:\n" + strings.TrimSpace(wake.Reason)
	}
	run, err := session.Submit(ctx, engine.RunRequest{Prompt: prompt})
	if err != nil {
		logger.Error().Err(err).Str("stage", "submit").Msg("")
		return Result{}, err
	}
	text := ""
	for event := range run.Events {
		if event.Kind == engine.EventMessageCompleted && event.Message != nil {
			if value := strings.TrimSpace(event.Message.Text()); value != "" {
				text = value
			}
		}
	}
	result := <-run.Done
	if result.Err != nil {
		logger.Error().Err(result.Err).Str("stage", "done").Msg("")
		return Result{}, result.Err
	}
	if text == "" {
		for i := len(result.State.Messages) - 1; i >= 0; i-- {
			if value := strings.TrimSpace(result.State.Messages[i].Text()); value != "" {
				text = value
				break
			}
		}
	}
	interpreted := Interpret(text)
	logger.Info().Bool("actionable", interpreted.Actionable).Int("text_len", len(strings.TrimSpace(interpreted.Text))).Msg("")
	return interpreted, nil
}
