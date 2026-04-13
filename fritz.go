package fritz

import (
	"context"

	"fritz/internal/auth"
	"fritz/internal/authstore"
	"fritz/internal/config"
	"fritz/internal/engine"
	"fritz/internal/gemini"
	"fritz/internal/model"
	"fritz/internal/openaicodex"
	"fritz/internal/provider"
	"fritz/internal/tool"
)

type Event = engine.Event
type EventKind = engine.EventKind
type Run = engine.Run
type RunRequest = engine.RunRequest
type RunResult = engine.RunResult
type Session = engine.Session
type SessionOptions = engine.SessionOptions
type SessionRef = engine.SessionRef
type Service = engine.Service
type LocalService = engine.LocalService

type Runtime = config.Runtime
type Source = config.Source
type Sources = config.Sources

type Client = model.Client
type ClientFactory = engine.ClientFactory
type Registry = tool.Registry
type RegistryFactory = engine.RegistryFactory
type ToolCall = tool.Call
type ToolResult = tool.Result
type ToolContentPart = tool.ContentPart

const (
	EventRunStarted         = engine.EventRunStarted
	EventStepStarted        = engine.EventStepStarted
	EventReasoningStarted   = engine.EventReasoningStarted
	EventReasoningDelta     = engine.EventReasoningDelta
	EventReasoningCompleted = engine.EventReasoningCompleted
	EventTextDelta          = engine.EventTextDelta
	EventMessageCompleted   = engine.EventMessageCompleted
	EventToolCallStarted    = engine.EventToolCallStarted
	EventToolCallCompleted  = engine.EventToolCallCompleted
	EventStepFinished       = engine.EventStepFinished
	EventRunFinished        = engine.EventRunFinished
	EventRunFailed          = engine.EventRunFailed
	EventRunCanceled        = engine.EventRunCanceled
)

func DefaultSource() Source {
	return config.DefaultSource()
}

func LoadEnv() Source {
	return config.LoadEnv()
}

func LoadForDir(dir string, overridePath string) (Source, string, error) {
	return config.LoadForDir(dir, overridePath)
}

func ResolveConfig(s Sources) Runtime {
	return config.Resolve(s)
}

func NewLocalService(
	cwd string,
	cfg Runtime,
	newClient ClientFactory,
	newRegistry RegistryFactory,
) *LocalService {
	return engine.NewLocalService(cwd, cfg, newClient, newRegistry)
}

func NewDefaultService(cwd string, cfg Runtime) *LocalService {
	return engine.NewLocalService(
		cwd,
		cfg,
		func(cfg Runtime) model.Client {
			switch cfg.Provider {
			case provider.OpenAICodex:
				resolver := auth.NewResolver(authstore.NewGlobalFileStore())
				return openaicodex.NewClient(
					func(ctx context.Context) (provider.RequestAuth, error) {
						return resolver.Resolve(ctx, cfg)
					},
					openaicodex.WithEndpoint(cfg.OpenAICodexEndpoint),
					openaicodex.WithModel(cfg.ModelID),
				)
			default:
				return gemini.NewClient(
					cfg.GeminiAPIKey,
					gemini.WithEndpoint(cfg.GeminiEndpoint),
					gemini.WithModel(cfg.ModelID),
				)
			}
		},
		func(cfg Runtime) *tool.Registry {
			registry := tool.NewRegistry()
			registry.Register(tool.NewBashTool(cwd, tool.WithDefaultTimeout(cfg.CommandTimeout)))
			registry.Register(tool.NewEditTool(cwd, 128*1024))
			registry.Register(tool.NewFindTool(cwd))
			registry.Register(tool.NewGrepTool(cwd))
			registry.Register(tool.NewLsTool(cwd))
			registry.Register(tool.NewReadTool(cwd, 128*1024))
			registry.Register(tool.NewWebSearchTool(cfg.GeminiAPIKey, cfg.GeminiEndpoint))
			registry.Register(tool.NewWriteTool(cwd))
			return registry
		},
	)
}

func Start(
	ctx context.Context,
	cwd string,
	cfg Runtime,
	newClient ClientFactory,
	newRegistry RegistryFactory,
	options SessionOptions,
) (Session, error) {
	return NewLocalService(cwd, cfg, newClient, newRegistry).Start(ctx, options)
}

func StartDefault(
	ctx context.Context,
	cwd string,
	cfg Runtime,
	options SessionOptions,
) (Session, error) {
	return NewDefaultService(cwd, cfg).Start(ctx, options)
}

func NewRegistryFactory(registry *Registry) RegistryFactory {
	return engine.NewRegistryFactory(registry)
}

func NewClientFactory(factory func(Runtime) Client) ClientFactory {
	return engine.NewClientFactory(factory)
}
