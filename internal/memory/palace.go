package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fritz/pkg/memorypalace"
)

const durableMemorySyncKind = "durable-memory"

func IndexDocuments(ctx context.Context, cwd string, engine *memorypalace.Engine) error {
	if engine == nil {
		return fmt.Errorf("%w: engine required", memorypalace.ErrInvalidRequest)
	}
	docs, err := Load(cwd)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		source := toSyncSource(cwd, doc)
		seen[source.ExternalRef] = struct{}{}
		if _, err := engine.SyncDocument(ctx, source); err != nil {
			return err
		}
	}
	existing, err := engine.ListSourceSyncByKind(ctx, durableMemorySyncKind)
	if err != nil {
		return err
	}
	for _, sync := range existing {
		if _, ok := seen[sync.ExternalRef]; ok {
			continue
		}
		if err := engine.MarkMissing(ctx, durableMemorySyncKind, sync.ExternalRef, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func SearchDocuments(ctx context.Context, engine *memorypalace.Engine, query string, limit int) ([]memorypalace.SearchHit, error) {
	if engine == nil {
		return nil, fmt.Errorf("%w: engine required", memorypalace.ErrInvalidRequest)
	}
	return engine.Search(ctx, memorypalace.SearchRequest{
		Query: query,
		Limit: limit,
	})
}

func toSyncSource(cwd string, doc Document) memorypalace.SyncSource {
	rel := doc.Path
	if relPath, err := filepath.Rel(cwd, doc.Path); err == nil {
		rel = filepath.ToSlash(relPath)
	}
	room := "general"
	if strings.Contains(rel, "/") {
		room = "journal"
	}
	version := fileVersion(doc.Path)
	return memorypalace.SyncSource{
		SourceID:      stableID("src", rel),
		SyncKind:      durableMemorySyncKind,
		ExternalRef:   doc.Path,
		Path:          rel,
		Title:         filepath.Base(rel),
		Scope:         cwd,
		SourceKind:    "memory-file",
		SourceVersion: version,
		Content:       doc.Content,
		Metadata: map[string]string{
			"path": rel,
			"wing": "durable-memory",
			"room": room,
		},
	}
}

func fileVersion(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UTC().UnixNano(), info.Size())
}

func stableID(prefix string, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "-" + hex.EncodeToString(sum[:8])
}
