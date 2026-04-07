package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

type readTool struct {
	root     string
	maxBytes int
	ops      FileOperations
}

type ReadToolOption func(*readTool)

func WithReadFileOperations(ops FileOperations) ReadToolOption {
	return func(t *readTool) {
		t.ops = ops
	}
}

func NewReadTool(root string, maxBytes int, options ...ReadToolOption) Tool {
	if maxBytes == 0 {
		maxBytes = DefaultReadMaxBytes
	}
	tool := readTool{root: root, maxBytes: maxBytes, ops: CreateLocalFileOperations()}
	for _, option := range options {
		option(&tool)
	}
	return tool
}

func (t readTool) Definition() Definition {
	return Definition{
		Name:          "read",
		Description:   "Read file contents. Supports text files and images.",
		PromptSnippet: "Read file contents",
		PromptGuidelines: []string{
			"Use read to inspect files instead of shelling out for simple file reads.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"path":   {Type: "string", Description: "Path to the file to read"},
				"offset": {Type: "number", Description: "1-indexed line offset"},
				"limit":  {Type: "number", Description: "Maximum number of lines to read"},
			},
			Required: []string{"path"},
		},
	}
}

func (t readTool) Run(ctx context.Context, call Call) (Result, error) {
	rawPath, errResult, err := requireStringArg(t.root, call, "path")
	if err != nil {
		return errResult, err
	}
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	resolved := ResolveReadPath(rawPath, t.root)
	info, err := t.ops.Stat(resolved)
	if err != nil {
		return errorResult(call, err), err
	}
	if info.IsDir() {
		err := fmt.Errorf("path %q is a directory", rawPath)
		return errorResult(call, err), err
	}

	data, err := t.ops.ReadFile(resolved)
	if err != nil {
		return errorResult(call, err), err
	}
	if mimeType := detectSupportedImageMimeType(data); mimeType != "" {
		return Result{
			CallID: call.ID,
			Name:   call.Name,
			Parts: []ContentPart{
				TextPart(fmt.Sprintf("Read image file [%s]", mimeType)),
				ImagePart(mimeType, base64.StdEncoding.EncodeToString(data)),
			},
		}, nil
	}

	textContent := string(data)
	lines := strings.Split(textContent, "\n")
	totalLines := len(lines)
	offset := intArg(call.Args["offset"])
	if offset < 1 {
		offset = 1
	}
	start := offset - 1
	if start >= len(lines) {
		err := fmt.Errorf("Offset %d is beyond end of file (%d lines total)", offset, len(lines))
		return errorResult(call, err), err
	}
	limit := intArg(call.Args["limit"])
	selected := lines[start:]
	userLimitedLines := 0
	if limit > 0 && limit < len(selected) {
		selected = selected[:limit]
		userLimitedLines = len(selected)
	}
	selectedContent := strings.Join(selected, "\n")
	truncation := TruncateHead(selectedContent, TruncationOptions{})
	output := truncation.Content
	var details *ReadResultDetails

	if truncation.FirstLineExceedsLimit {
		output = fmt.Sprintf("[Line %d is %s, exceeds %s limit. Use bash fallback.]", offset, FormatSize(len([]byte(lines[start]))), FormatSize(DefaultMaxBytes))
		details = &ReadResultDetails{Truncation: &truncation}
	} else if truncation.Truncated {
		endLine := offset + truncation.OutputLines - 1
		nextOffset := endLine + 1
		if truncation.TruncatedBy == "lines" {
			output += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]", offset, endLine, totalLines, nextOffset)
		} else {
			output += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]", offset, endLine, totalLines, FormatSize(DefaultMaxBytes), nextOffset)
		}
		details = &ReadResultDetails{Truncation: &truncation}
	} else if userLimitedLines > 0 && start+userLimitedLines < len(lines) {
		remaining := len(lines) - (start + userLimitedLines)
		nextOffset := start + userLimitedLines + 1
		output += fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", remaining, nextOffset)
	}

	var resultDetails any
	if details != nil {
		resultDetails = details
	}
	return Result{
		CallID:  call.ID,
		Name:    call.Name,
		Parts:   []ContentPart{TextPart(output)},
		Details: resultDetails,
	}, nil
}
