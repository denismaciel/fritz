package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type GrepBackend string

const (
	GrepBackendRipgrep GrepBackend = "ripgrep"
	GrepBackendGo      GrepBackend = "go"
)

type grepTool struct {
	root    string
	ops     FileOperations
	backend GrepBackend
	rgPath  string
}

type findTool struct {
	root string
	ops  FileOperations
}

type lsTool struct {
	root string
	ops  FileOperations
}

type DiscoveryToolOption func(*discoveryToolOptions)

type discoveryToolOptions struct {
	ops         FileOperations
	grepBackend GrepBackend
	rgPath      string
}

func WithDiscoveryFileOperations(ops FileOperations) DiscoveryToolOption {
	return func(o *discoveryToolOptions) {
		o.ops = ops
	}
}

func WithGrepBackend(backend GrepBackend) DiscoveryToolOption {
	return func(o *discoveryToolOptions) {
		o.grepBackend = backend
	}
}

func WithRipgrepPath(path string) DiscoveryToolOption {
	return func(o *discoveryToolOptions) {
		o.rgPath = path
	}
}

func newDiscoveryToolOptions(options []DiscoveryToolOption) discoveryToolOptions {
	out := discoveryToolOptions{ops: CreateLocalFileOperations(), grepBackend: GrepBackendRipgrep}
	for _, option := range options {
		option(&out)
	}
	return out
}

func NewGrepTool(root string, options ...DiscoveryToolOption) Tool {
	cfg := newDiscoveryToolOptions(options)
	return grepTool{root: root, ops: cfg.ops, backend: cfg.grepBackend, rgPath: cfg.rgPath}
}

func NewFindTool(root string, options ...DiscoveryToolOption) Tool {
	cfg := newDiscoveryToolOptions(options)
	return findTool{root: root, ops: cfg.ops}
}

func NewLsTool(root string, options ...DiscoveryToolOption) Tool {
	cfg := newDiscoveryToolOptions(options)
	return lsTool{root: root, ops: cfg.ops}
}

func (t grepTool) Definition() Definition {
	return Definition{
		Name:          "grep",
		Description:   "Search file contents for patterns.",
		PromptSnippet: "Search file contents by pattern",
		PromptGuidelines: []string{
			"Prefer grep over bash for repo text search.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"pattern":    {Type: "string"},
				"path":       {Type: "string"},
				"glob":       {Type: "string"},
				"ignoreCase": {Type: "boolean"},
				"literal":    {Type: "boolean"},
				"context":    {Type: "number"},
				"limit":      {Type: "number"},
			},
			Required: []string{"pattern"},
		},
	}
}

func (t grepTool) Run(ctx context.Context, call Call) (Result, error) {
	pattern, ok := call.Args["pattern"].(string)
	if !ok || pattern == "" {
		err := errors.New("missing required arg: pattern")
		return errorResult(call, err), err
	}
	searchPath := "."
	if value, ok := call.Args["path"].(string); ok && value != "" {
		searchPath = value
	}
	resolved, errResult, err := requirePathWithinRoot(t.root, call, searchPath)
	if err != nil {
		return errResult, err
	}
	if t.backend == GrepBackendGo {
		return t.runGoGrep(call, resolved)
	}
	return t.runRipgrep(ctx, call, resolved)
}

func (t findTool) Definition() Definition {
	return Definition{
		Name:          "find",
		Description:   "Search for files by glob pattern.",
		PromptSnippet: "Find files by glob pattern",
		PromptGuidelines: []string{
			"Prefer find over bash for file discovery.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"pattern": {Type: "string"},
				"path":    {Type: "string"},
				"limit":   {Type: "number"},
			},
			Required: []string{"pattern"},
		},
	}
}

func (t findTool) Run(_ context.Context, call Call) (Result, error) {
	pattern, ok := call.Args["pattern"].(string)
	if !ok || pattern == "" {
		err := errors.New("missing required arg: pattern")
		return errorResult(call, err), err
	}
	searchPath := "."
	if value, ok := call.Args["path"].(string); ok && value != "" {
		searchPath = value
	}
	resolved, errResult, err := requirePathWithinRoot(t.root, call, searchPath)
	if err != nil {
		return errResult, err
	}
	limit := intArg(call.Args["limit"])
	if limit <= 0 {
		limit = 1000
	}

	var results []string
	err = t.ops.WalkDir(resolved, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == resolved {
			return nil
		}
		if shouldIgnore(t.ops, path, resolved, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(resolved, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if globMatch(pattern, rel) || globMatch(pattern, filepath.Base(rel)) {
			if d.IsDir() {
				rel += "/"
			}
			results = append(results, rel)
			if len(results) >= limit {
				return errStopWalk
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return errorResult(call, err), err
	}
	if len(results) == 0 {
		return TextResult(call, "No files found matching pattern"), nil
	}

	sort.Strings(results)
	rawOutput := strings.Join(results, "\n")
	truncation := TruncateHead(rawOutput, TruncationOptions{MaxLines: 1 << 20})
	output := truncation.Content
	var details *FindResultDetails
	if len(results) >= limit {
		details = &FindResultDetails{ResultLimitReached: limit}
		output += fmt.Sprintf("\n\n[%d results limit reached]", limit)
	}
	if truncation.Truncated {
		if details == nil {
			details = &FindResultDetails{}
		}
		details.Truncation = &truncation
	}
	var resultDetails any
	if details != nil {
		resultDetails = details
	}
	return Result{CallID: call.ID, Name: call.Name, Parts: []ContentPart{TextPart(output)}, Details: resultDetails}, nil
}

func (t lsTool) Definition() Definition {
	return Definition{
		Name:          "ls",
		Description:   "List directory contents.",
		PromptSnippet: "List directory contents",
		PromptGuidelines: []string{
			"Prefer ls over bash for directory listing.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"path":  {Type: "string"},
				"limit": {Type: "number"},
			},
		},
	}
}

func (t lsTool) Run(_ context.Context, call Call) (Result, error) {
	searchPath := "."
	if value, ok := call.Args["path"].(string); ok && value != "" {
		searchPath = value
	}
	resolved, errResult, err := requirePathWithinRoot(t.root, call, searchPath)
	if err != nil {
		return errResult, err
	}
	info, err := t.ops.Stat(resolved)
	if err != nil {
		return errorResult(call, err), err
	}
	if !info.IsDir() {
		err := fmt.Errorf("Not a directory: %s", resolved)
		return errorResult(call, err), err
	}
	limit := intArg(call.Args["limit"])
	if limit <= 0 {
		limit = 500
	}
	entries, err := t.ops.ReadDir(resolved)
	if err != nil {
		return errorResult(call, err), err
	}
	sort.Slice(entries, func(i int, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	lines := make([]string, 0, min(len(entries), limit))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
		if len(lines) >= limit {
			break
		}
	}
	if len(lines) == 0 {
		return TextResult(call, "(empty directory)"), nil
	}
	rawOutput := strings.Join(lines, "\n")
	truncation := TruncateHead(rawOutput, TruncationOptions{MaxLines: 1 << 20})
	output := truncation.Content
	var details *LsResultDetails
	if len(entries) > limit {
		details = &LsResultDetails{EntryLimitReached: limit}
		output += fmt.Sprintf("\n\n[%d entries limit reached]", limit)
	}
	if truncation.Truncated {
		if details == nil {
			details = &LsResultDetails{}
		}
		details.Truncation = &truncation
	}
	var resultDetails any
	if details != nil {
		resultDetails = details
	}
	return Result{CallID: call.ID, Name: call.Name, Parts: []ContentPart{TextPart(output)}, Details: resultDetails}, nil
}
