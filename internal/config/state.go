package config

import (
	"os"
	"path/filepath"
	"strings"
)

func GlobalStateRoot() string {
	if path := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); path != "" {
		return filepath.Join(path, "fritz")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "fritz")
}

func WorkspacesStateRoot() string {
	root := GlobalStateRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "workspaces")
}

func WorkspaceKey(cwd string) string {
	clean := filepath.Clean(cwd)
	clean = filepath.ToSlash(clean)
	clean = strings.ReplaceAll(clean, ":", "")
	return strings.ReplaceAll(clean, "/", "--")
}

func WorkspaceStateRoot(cwd string) string {
	return filepath.Join(WorkspacesStateRoot(), WorkspaceKey(cwd))
}

func DefaultWorkspaceSessionDir(cwd string) string {
	return filepath.Join(WorkspaceStateRoot(cwd), "sessions")
}

func DefaultWorkspaceLogFile(cwd string) string {
	return filepath.Join(WorkspaceStateRoot(cwd), "logs", "agent.jsonl")
}

func DefaultWorkspaceGatewayRoot(cwd string) string {
	return filepath.Join(WorkspaceStateRoot(cwd), "gateway")
}

func DefaultWorkspaceSecretsFile(cwd string) string {
	return filepath.Join(WorkspaceStateRoot(cwd), "secrets.json")
}

func DefaultWorkspaceAuthFile(cwd string) string {
	return filepath.Join(WorkspaceStateRoot(cwd), "auth.json")
}

func DefaultWorkspaceAuthLockFile(cwd string) string {
	return filepath.Join(WorkspaceStateRoot(cwd), "auth.lock")
}

func ResolveSessionDir(cwd string, sessionRoot string) (string, error) {
	base := strings.TrimSpace(sessionRoot)
	if base == "" {
		return DefaultWorkspaceSessionDir(cwd), nil
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(cwd, base)
	}
	return filepath.Join(base, WorkspaceKey(cwd)), nil
}

func ResolveLogFile(cwd string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return DefaultWorkspaceLogFile(cwd)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}
