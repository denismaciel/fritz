package memory

import (
	"os"
	"path/filepath"
	"sort"
)

type Document struct {
	Path    string
	Content string
}

type Paths struct {
	Root     string
	MainPath string
	Dir      string
}

func ResolvePaths(cwd string) Paths {
	root := cwd
	return Paths{
		Root:     root,
		MainPath: filepath.Join(root, "MEMORY.md"),
		Dir:      filepath.Join(root, "memory"),
	}
}

func Load(cwd string) ([]Document, error) {
	paths := ResolvePaths(cwd)
	var docs []Document
	if data, err := os.ReadFile(paths.MainPath); err == nil {
		docs = append(docs, Document{
			Path:    paths.MainPath,
			Content: string(data),
		})
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	entries, err := os.ReadDir(paths.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return docs, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(paths.Dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		docs = append(docs, Document{
			Path:    path,
			Content: string(data),
		})
	}
	return docs, nil
}
