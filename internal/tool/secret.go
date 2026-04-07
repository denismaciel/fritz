package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fritz/internal/secretstore"
)

type secretSetTool struct {
	store *secretstore.Store
}

type secretListTool struct {
	store *secretstore.Store
}

type secretDeleteTool struct {
	store *secretstore.Store
}

func NewSecretSetTool(cwd string) Tool {
	return secretSetTool{store: secretstore.New(cwd)}
}

func NewSecretListTool(cwd string) Tool {
	return secretListTool{store: secretstore.New(cwd)}
}

func NewSecretDeleteTool(cwd string) Tool {
	return secretDeleteTool{store: secretstore.New(cwd)}
}

func (t secretSetTool) Definition() Definition {
	return Definition{
		Name:          "secret_set",
		Description:   "Store a named secret in harness-owned secret storage.",
		PromptSnippet: "Store named secrets outside memory files",
		PromptGuidelines: []string{
			"Do not write API keys, tokens, or passwords to MEMORY.md, HEARTBEAT.md, or other workspace docs.",
			"Use secret_set for durable secrets the harness should retain.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"name":  {Type: "string", Description: "Secret name like strava.api_key"},
				"value": {Type: "string", Description: "Secret value"},
			},
			Required: []string{"name", "value"},
		},
	}
}

func (t secretSetTool) Run(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	name, errResult, err := requireStringArg("", call, "name")
	if err != nil {
		return errResult, err
	}
	value, ok := call.Args["value"].(string)
	if !ok || value == "" {
		err := errors.New("missing required arg: value")
		return errorResult(call, err), err
	}
	if err := t.store.Set(name, value); err != nil {
		return errorResult(call, err), err
	}
	return TextResult(call, fmt.Sprintf("Stored secret %q.", name)), nil
}

func (t secretListTool) Definition() Definition {
	return Definition{
		Name:          "secret_list",
		Description:   "List stored secret names without revealing values.",
		PromptSnippet: "List stored secret names only",
		PromptGuidelines: []string{
			"Use secret_list to inspect which named secrets exist without exposing values.",
		},
		Parameters: Parameters{
			Type:       "object",
			Properties: map[string]Property{},
		},
	}
}

func (t secretListTool) Run(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	items, err := t.store.List()
	if err != nil {
		return errorResult(call, err), err
	}
	if len(items) == 0 {
		return TextResult(call, "No stored secrets."), nil
	}
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, "Stored secrets:")
	for _, item := range items {
		line := "- " + item.Name
		if item.UpdatedAt != "" {
			line += " (" + item.UpdatedAt + ")"
		}
		lines = append(lines, line)
	}
	return TextResult(call, strings.Join(lines, "\n")), nil
}

func (t secretDeleteTool) Definition() Definition {
	return Definition{
		Name:          "secret_delete",
		Description:   "Delete a stored secret by name.",
		PromptSnippet: "Delete stored secrets by name",
		PromptGuidelines: []string{
			"Use secret_delete only when asked to remove a stored secret or when rotating credentials.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"name": {Type: "string", Description: "Secret name"},
			},
			Required: []string{"name"},
		},
	}
}

func (t secretDeleteTool) Run(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	name, errResult, err := requireStringArg("", call, "name")
	if err != nil {
		return errResult, err
	}
	deleted, err := t.store.Delete(name)
	if err != nil {
		return errorResult(call, err), err
	}
	if !deleted {
		return TextResult(call, fmt.Sprintf("Secret %q not found.", name)), nil
	}
	return TextResult(call, fmt.Sprintf("Deleted secret %q.", name)), nil
}
