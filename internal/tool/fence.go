package tool

import (
	"context"
	"fmt"
	"path/filepath"

	fence "github.com/Use-Tusk/fence/pkg/fence"
)

type fenceBashOperations struct {
	workspace WorkspaceConfig
	local     BashOperations
}

func NewFenceBashOperations(workspace WorkspaceConfig) BashOperations {
	return fenceBashOperations{
		workspace: workspace,
		local:     CreateLocalBashOperations(),
	}
}

func (o fenceBashOperations) Exec(ctx context.Context, command string, cwd string, options BashExecOptions) (BashExecResult, error) {
	if !o.workspace.Enabled() {
		return BashExecResult{}, fmt.Errorf("fence sandbox requires workspace root")
	}
	root := filepath.Clean(o.workspace.Root)
	cfg := fence.DefaultConfig()
	cfg.Network.AllowedDomains = []string{"*"}
	allowLocalOutbound := false
	cfg.Network.AllowLocalOutbound = &allowLocalOutbound
	cfg.Filesystem.DefaultDenyRead = true
	cfg.Filesystem.AllowRead = []string{root}
	cfg.Filesystem.AllowWrite = []string{root}
	for _, readOnly := range o.workspace.ReadOnlyPaths {
		cfg.Filesystem.DenyWrite = append(cfg.Filesystem.DenyWrite, filepath.Join(root, filepath.FromSlash(readOnly)))
	}

	manager := fence.NewManager(cfg, false, false)
	defer manager.Cleanup()
	if err := manager.ExposeHostPath(root, true); err != nil {
		return BashExecResult{}, err
	}
	wrapped, err := manager.WrapCommandInDir(command, root)
	if err != nil {
		return BashExecResult{}, err
	}

	env := workspaceEnv(o.workspace)
	if options.Env != nil {
		env = options.Env
	}
	options.Env = env
	return o.local.Exec(ctx, wrapped, root, options)
}

func workspaceEnv(workspace WorkspaceConfig) map[string]string {
	root := filepath.Clean(workspace.Root)
	home := filepath.Join(root, filepath.FromSlash(workspace.homeDir()))
	tmp := filepath.Join(root, filepath.FromSlash(workspace.tempDir()))
	env := map[string]string{
		"HOME":   home,
		"PWD":    root,
		"TMPDIR": tmp,
		"LANG":   "C.UTF-8",
		"TERM":   "dumb",
		"PATH":   "/usr/local/bin:/usr/bin:/bin",
	}
	for key, value := range workspace.Env {
		env[key] = value
	}
	return env
}
