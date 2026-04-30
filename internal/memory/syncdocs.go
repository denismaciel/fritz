package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fritz/pkg/memorypalace"
)

var syncDocExtensions = []string{
	".md",
	".txt",
	".go",
	".py",
	".js",
	".ts",
	".json",
	".yaml",
	".yml",
	".sql",
	".toml",
}

type SyncDocFile struct {
	AbsolutePath string
	DisplayPath  string
	Title        string
	SourceKind   string
	Scope        string
	Metadata     map[string]string
}

func SyncFilesystemDocuments(ctx context.Context, root string, engine *memorypalace.Engine) ([]memorypalace.SyncResult, error) {
	if engine == nil {
		return nil, fmt.Errorf("%w: engine required", memorypalace.ErrInvalidRequest)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	files, err := scanSyncDocFiles(root)
	if err != nil {
		return nil, err
	}
	results, seen, err := syncDocFiles(ctx, engine, "filesystem", root, files)
	if err != nil {
		return nil, err
	}
	existing, err := engine.ListSourceSyncByKind(ctx, "filesystem")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, sync := range existing {
		if !pathWithinRoot(root, sync.ExternalRef) {
			continue
		}
		if _, ok := seen[sync.ExternalRef]; ok {
			continue
		}
		if err := engine.MarkMissing(ctx, "filesystem", sync.ExternalRef, now); err != nil {
			return nil, err
		}
		results = append(results, memorypalace.SyncResult{
			SourceID:    sync.SourceID,
			Action:      "missing",
			ContentHash: sync.ContentHash,
		})
	}
	return results, nil
}

func scanSyncDocFiles(root string) ([]SyncDocFile, error) {
	var files []SyncDocFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isSyncDocExtension(filepath.Ext(d.Name())) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files = append(files, SyncDocFile{
			AbsolutePath: path,
			DisplayPath:  rel,
			Title:        filepath.Base(rel),
			SourceKind:   "document",
			Scope:        root,
			Metadata: map[string]string{
				"path": rel,
				"wing": "filesystem",
				"room": "general",
			},
		})
		return nil
	})
	return files, err
}

func syncDocFiles(ctx context.Context, engine *memorypalace.Engine, syncKind string, scope string, files []SyncDocFile) ([]memorypalace.SyncResult, map[string]struct{}, error) {
	results := make([]memorypalace.SyncResult, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		content, err := os.ReadFile(file.AbsolutePath)
		if err != nil {
			return nil, nil, err
		}
		seen[file.AbsolutePath] = struct{}{}
		result, err := engine.SyncDocument(ctx, memorypalace.SyncSource{
			SyncKind:      syncKind,
			ExternalRef:   file.AbsolutePath,
			Path:          file.DisplayPath,
			Title:         defaultTitle(file.Title, filepath.Base(file.DisplayPath)),
			Scope:         defaultTitle(file.Scope, scope),
			SourceKind:    defaultTitle(file.SourceKind, "document"),
			SourceVersion: fileVersion(file.AbsolutePath),
			Content:       string(content),
			Metadata:      cloneMetadata(file.Metadata),
			SeenAt:        time.Now().UTC(),
		})
		if err != nil {
			return nil, nil, err
		}
		results = append(results, result)
	}
	return results, seen, nil
}

func isSyncDocExtension(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	return ext != "" && slices.Contains(syncDocExtensions, ext)
}

func pathWithinRoot(root string, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

func defaultTitle(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
