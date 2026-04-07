package session

import (
	"context"
	"fmt"

	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/model"
)

type StartOptions struct {
	Continue    bool
	SessionPath string
	ForkPath    string
	NoSession   bool
	NewSession  bool
}

type Runtime struct {
	Manager *Manager
	State   chat.State
	Config  config.Runtime
}

func Start(ctx context.Context, cwd string, cfg config.Runtime, options StartOptions) (Runtime, error) {
	_ = ctx
	if !cfg.Session.Enabled || options.NoSession {
		return Runtime{Manager: InMemory(cwd), State: chat.NewState(), Config: cfg}, nil
	}

	var (
		manager *Manager
		err     error
	)
	switch {
	case options.ForkPath != "":
		manager, err = ForkFrom(options.ForkPath, cwd, cfg.Session.Dir)
	case options.SessionPath != "":
		manager, err = Open(options.SessionPath, cfg.Session.Dir)
	case options.Continue && !options.NewSession:
		manager, err = ContinueRecent(cwd, cfg.Session.Dir)
	default:
		manager, err = Create(cwd, cfg.Session.Dir)
	}
	if err != nil {
		return Runtime{}, err
	}
	context := manager.BuildContext()
	return Runtime{
		Manager: manager,
		State: chat.State{
			Transcript: context.Transcript,
			Messages:   context.Messages,
		},
		Config: cfg,
	}, nil
}

type Host struct {
	cwd   string
	cfg   config.Runtime
	state Runtime
}

func NewHost(cwd string, cfg config.Runtime, state Runtime) *Host {
	return &Host{cwd: cwd, cfg: cfg, state: state}
}

func (h *Host) Runtime() Runtime {
	return h.state
}

func (h *Host) NewSession(ctx context.Context, parentSession string) error {
	options := StartOptions{NewSession: true}
	runtime, err := Start(ctx, h.cwd, h.cfg, options)
	if err != nil {
		return err
	}
	if parentSession != "" {
		header := runtime.Manager.lines[0]
		header.ParentSession = parentSession
		runtime.Manager.lines[0] = header
		if runtime.Manager.persist {
			if err := rewriteLines(runtime.Manager.sessionFile, runtime.Manager.lines); err != nil {
				return err
			}
		}
	}
	h.state = runtime
	return nil
}

func (h *Host) SwitchSession(ctx context.Context, path string) error {
	runtime, err := Start(ctx, h.cwd, h.cfg, StartOptions{SessionPath: path})
	if err != nil {
		return err
	}
	h.state = runtime
	return nil
}

func (h *Host) Fork(ctx context.Context, path string) error {
	runtime, err := Start(ctx, h.cwd, h.cfg, StartOptions{ForkPath: path})
	if err != nil {
		return err
	}
	h.state = runtime
	return nil
}

func (h *Host) ImportFromJSONL(ctx context.Context, path string) error {
	return h.SwitchSession(ctx, path)
}

func NavigateTree(ctx context.Context, manager *Manager, targetID string, summarize bool, gateway model.Gateway, modelID string) (Context, error) {
	_ = ctx
	oldLeaf := manager.LeafID()
	if !summarize {
		if err := manager.MoveLeaf(targetID); err != nil {
			return Context{}, err
		}
		return manager.BuildContext(), nil
	}
	summary, err := GenerateBranchSummary(context.Background(), manager, oldLeaf, targetID, gateway, modelID)
	if err != nil {
		return Context{}, err
	}
	if _, err := manager.BranchWithSummary(targetID, summary); err != nil {
		return Context{}, err
	}
	return manager.BuildContext(), nil
}

func (r Runtime) SessionStats() SessionInfo {
	if r.Manager == nil {
		return SessionInfo{}
	}
	return r.Manager.Stats()
}

func (r Runtime) RequireManager() (*Manager, error) {
	if r.Manager == nil {
		return nil, fmt.Errorf("missing session manager")
	}
	return r.Manager, nil
}
