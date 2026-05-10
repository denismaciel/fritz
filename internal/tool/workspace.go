package tool

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type CommandSandbox string

const (
	CommandSandboxLocal CommandSandbox = ""
	CommandSandboxFence CommandSandbox = "fence"
)

type WorkspaceConfig struct {
	Root           string
	ReadOnlyPaths  []string
	Env            map[string]string
	HomeDir        string
	TempDir        string
	SpillDir       string
	CommandSandbox CommandSandbox
}

func (c WorkspaceConfig) Enabled() bool {
	return strings.TrimSpace(c.Root) != ""
}

func (c WorkspaceConfig) rootClean() string {
	return filepath.Clean(c.Root)
}

func (c WorkspaceConfig) homeDir() string {
	if strings.TrimSpace(c.HomeDir) != "" {
		return c.HomeDir
	}
	return ".home"
}

func (c WorkspaceConfig) tempDir() string {
	if strings.TrimSpace(c.TempDir) != "" {
		return c.TempDir
	}
	return ".tmp"
}

func (c WorkspaceConfig) spillDir() string {
	if strings.TrimSpace(c.SpillDir) != "" {
		return c.SpillDir
	}
	return filepath.Join(".fritz", "spills")
}

func (c WorkspaceConfig) ResolvedSpillDir() string {
	if !c.Enabled() {
		return ""
	}
	return filepath.Join(c.rootClean(), filepath.FromSlash(c.spillDir()))
}

func NewWorkspaceFileOperations(cfg WorkspaceConfig) FileOperations {
	return workspaceFileOperations{cfg: cfg, ops: CreateLocalFileOperations()}
}

type workspaceFileOperations struct {
	cfg WorkspaceConfig
	ops FileOperations
}

func (o workspaceFileOperations) Stat(name string) (os.FileInfo, error) {
	path, err := o.safeExistingPath(name)
	if err != nil {
		return nil, err
	}
	return o.ops.Stat(path)
}

func (o workspaceFileOperations) ReadFile(name string) ([]byte, error) {
	path, err := o.safeExistingPath(name)
	if err != nil {
		return nil, err
	}
	return o.ops.ReadFile(path)
}

func (o workspaceFileOperations) WriteFile(name string, data []byte, perm os.FileMode) error {
	path, err := o.safeWritePath(name)
	if err != nil {
		return err
	}
	return o.ops.WriteFile(path, data, perm)
}

func (o workspaceFileOperations) MkdirAll(path string, perm os.FileMode) error {
	resolved, err := o.safeMkdirAllPath(path)
	if err != nil {
		return err
	}
	return o.ops.MkdirAll(resolved, perm)
}

func (o workspaceFileOperations) ReadDir(name string) ([]os.DirEntry, error) {
	path, err := o.safeExistingPath(name)
	if err != nil {
		return nil, err
	}
	return o.ops.ReadDir(path)
}

func (o workspaceFileOperations) WalkDir(root string, fn fs.WalkDirFunc) error {
	path, err := o.safeExistingPath(root)
	if err != nil {
		return err
	}
	return o.ops.WalkDir(path, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fn(current, d, walkErr)
		}
		if err := o.ensureExistingPathWithinRoot(current); err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return fn(current, d, nil)
	})
}

func (o workspaceFileOperations) CreateTemp(dir string, pattern string) (*os.File, error) {
	if strings.TrimSpace(dir) == "" {
		dir = o.cfg.tempDir()
	}
	path, err := o.safeWritePath(dir)
	if err != nil {
		return nil, err
	}
	if err := o.ops.MkdirAll(path, 0o700); err != nil {
		return nil, err
	}
	return o.ops.CreateTemp(path, pattern)
}

func (o workspaceFileOperations) safeExistingPath(name string) (string, error) {
	path, err := o.resolvePath(name)
	if err != nil {
		return "", err
	}
	if err := o.ensureExistingPathWithinRoot(path); err != nil {
		return "", err
	}
	return path, nil
}

func (o workspaceFileOperations) safeWritePath(name string) (string, error) {
	path, err := o.resolvePath(name)
	if err != nil {
		return "", err
	}
	if o.isReadOnly(path) {
		return "", fmt.Errorf("path %q is read-only", o.displayPath(path))
	}
	parent := filepath.Dir(path)
	if err := o.ensureExistingPathWithinRoot(parent); err != nil {
		return "", err
	}
	return path, nil
}

func (o workspaceFileOperations) safeMkdirAllPath(name string) (string, error) {
	path, err := o.resolvePath(name)
	if err != nil {
		return "", err
	}
	if o.isReadOnly(path) {
		return "", fmt.Errorf("path %q is read-only", o.displayPath(path))
	}
	current := path
	for {
		if _, err := o.ops.Stat(current); err == nil {
			if err := o.ensureExistingPathWithinRoot(current); err != nil {
				return "", err
			}
			return path, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("path %q escapes workspace", o.displayPath(path))
		}
		current = parent
	}
}

func (o workspaceFileOperations) resolvePath(name string) (string, error) {
	if !o.cfg.Enabled() {
		return filepath.Clean(name), nil
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(name, "~") {
		return "", fmt.Errorf("home paths are not allowed: %q", name)
	}
	root := o.cfg.rootClean()
	path := filepath.Clean(name)
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("path %q escapes workspace", name)
		}
		return path, nil
	}
	rel := filepath.Clean(filepath.FromSlash(path))
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace", name)
	}
	return filepath.Join(root, rel), nil
}

func (o workspaceFileOperations) ensureExistingPathWithinRoot(path string) error {
	root := o.cfg.rootClean()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if !pathWithin(root, resolved) {
		return fmt.Errorf("path %q escapes workspace", o.displayPath(path))
	}
	return nil
}

func (o workspaceFileOperations) isReadOnly(path string) bool {
	root := o.cfg.rootClean()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	return slices.ContainsFunc(o.cfg.ReadOnlyPaths, func(readOnly string) bool {
		readOnly = filepath.ToSlash(filepath.Clean(filepath.FromSlash(readOnly)))
		return rel == readOnly || strings.HasPrefix(rel, readOnly+"/")
	})
}

func (o workspaceFileOperations) displayPath(path string) string {
	root := o.cfg.rootClean()
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}
