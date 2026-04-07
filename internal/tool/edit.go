package tool

import (
	"context"
	"fmt"
)

type editTool struct {
	root string
	ops  FileOperations
}

type EditToolOption func(*editTool)

func WithEditFileOperations(ops FileOperations) EditToolOption {
	return func(t *editTool) {
		t.ops = ops
	}
}

func NewEditTool(root string, _ int, options ...EditToolOption) Tool {
	tool := editTool{root: root, ops: CreateLocalFileOperations()}
	for _, option := range options {
		option(&tool)
	}
	return tool
}

func (t editTool) Definition() Definition {
	return Definition{
		Name:          "edit",
		Description:   "Edit a single file using exact text replacement.",
		PromptSnippet: "Edit files with exact text replacement",
		PromptGuidelines: []string{
			"Use edit for targeted modifications when old text can be matched exactly.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"path": {Type: "string", Description: "Path to edit"},
				"edits": {
					Type: "array",
					Items: &Property{
						Type: "object",
						Properties: map[string]Property{
							"oldText": {Type: "string", Description: "Old text"},
							"newText": {Type: "string", Description: "New text"},
						},
						Required: []string{"oldText", "newText"},
					},
				},
			},
			Required: []string{"path", "edits"},
		},
		PrepareArgs: PrepareEditArguments,
	}
}

func (t editTool) Run(ctx context.Context, call Call) (Result, error) {
	rawPath, errResult, err := requireStringArg(t.root, call, "path")
	if err != nil {
		return errResult, err
	}
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	resolved, errResult, err := requirePathWithinRoot(t.root, call, rawPath)
	if err != nil {
		return errResult, err
	}
	edits, err := parseEditArgs(call.Args)
	if err != nil {
		return errorResult(call, err), err
	}

	var result Result
	err = withFileMutationQueue(resolved, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := t.ops.ReadFile(resolved)
		if err != nil {
			return err
		}
		rawContent := string(data)
		bom, withoutBOM := StripBOM(rawContent)
		lineEnding := DetectLineEnding(withoutBOM)
		normalizedContent := NormalizeToLF(withoutBOM)
		baseContent, newContent, err := ApplyEditsToNormalizedContent(normalizedContent, edits, rawPath)
		if err != nil {
			return err
		}
		finalContent := bom + RestoreLineEndings(newContent, lineEnding)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.ops.WriteFile(resolved, []byte(finalContent), 0o644); err != nil {
			return err
		}
		diff, firstChangedLine := GenerateDiffString(baseContent, newContent)
		result = Result{
			CallID: call.ID,
			Name:   call.Name,
			Parts:  []ContentPart{TextPart(fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(edits), rawPath))},
			Details: EditResultDetails{
				Diff:             diff,
				FirstChangedLine: firstChangedLine,
			},
		}
		return nil
	})
	if err != nil {
		return errorResult(call, err), err
	}
	return result, nil
}

func parseEditArgs(args map[string]any) ([]Edit, error) {
	if rawEdits, ok := args["edits"]; ok {
		switch edits := rawEdits.(type) {
		case []Edit:
			if len(edits) == 0 {
				return nil, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
			}
			return edits, nil
		case []any:
			parsed := make([]Edit, 0, len(edits))
			for _, raw := range edits {
				editMap, ok := raw.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid edit entry")
				}
				parsed = append(parsed, Edit{
					OldText: stringValue(editMap["oldText"]),
					NewText: stringValue(editMap["newText"]),
				})
			}
			if len(parsed) == 0 {
				return nil, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
			}
			return parsed, nil
		case []map[string]any:
			parsed := make([]Edit, 0, len(edits))
			for _, editMap := range edits {
				parsed = append(parsed, Edit{
					OldText: stringValue(editMap["oldText"]),
					NewText: stringValue(editMap["newText"]),
				})
			}
			if len(parsed) == 0 {
				return nil, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
			}
			return parsed, nil
		}
	}
	oldText := stringValue(args["old_text"])
	if oldText == "" {
		oldText = stringValue(args["oldText"])
	}
	if oldText == "" {
		return nil, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
	}
	newText := stringValue(args["new_text"])
	if newText == "" && args["new_text"] == nil {
		newText = stringValue(args["newText"])
	}
	return []Edit{{OldText: oldText, NewText: newText}}, nil
}
