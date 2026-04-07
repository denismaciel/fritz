package tool

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

type writeTool struct {
	root string
	ops  FileOperations
}

type WriteToolOption func(*writeTool)

func WithWriteFileOperations(ops FileOperations) WriteToolOption {
	return func(t *writeTool) {
		t.ops = ops
	}
}

func NewWriteTool(root string, options ...WriteToolOption) Tool {
	tool := writeTool{root: root, ops: CreateLocalFileOperations()}
	for _, option := range options {
		option(&tool)
	}
	return tool
}

func (t writeTool) Definition() Definition {
	return Definition{
		Name:          "write",
		Description:   "Write full file contents.",
		PromptSnippet: "Create or overwrite files",
		PromptGuidelines: []string{
			"Use write for new files or full rewrites.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"path":    {Type: "string", Description: "Path to write"},
				"content": {Type: "string", Description: "Full file content"},
			},
			Required: []string{"path", "content"},
		},
	}
}

func (t writeTool) Run(ctx context.Context, call Call) (Result, error) {
	rawPath, errResult, err := requireStringArg(t.root, call, "path")
	if err != nil {
		return errResult, err
	}
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	content, ok := call.Args["content"].(string)
	if !ok {
		err := errors.New("missing required arg: content")
		return errorResult(call, err), err
	}
	resolved, errResult, err := requirePathWithinRoot(t.root, call, rawPath)
	if err != nil {
		return errResult, err
	}

	err = withFileMutationQueue(resolved, func() error {
		if err := t.ops.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			return err
		}
		return t.ops.WriteFile(resolved, []byte(content), 0o644)
	})
	if err != nil {
		return errorResult(call, err), err
	}
	return TextResult(call, fmt.Sprintf("Successfully wrote %d bytes to %s.", len(content), rawPath)), nil
}
