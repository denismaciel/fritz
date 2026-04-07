package tool

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"fritz/internal/logx"
)

type Property struct {
	Type        string              `json:"type"`
	Description string              `json:"description,omitempty"`
	Properties  map[string]Property `json:"properties,omitempty"`
	Required    []string            `json:"required,omitempty"`
	Items       *Property           `json:"items,omitempty"`
}

type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type Definition struct {
	Name             string              `json:"name"`
	Description      string              `json:"description,omitempty"`
	Parameters       Parameters          `json:"parameters"`
	PromptSnippet    string              `json:"promptSnippet,omitempty"`
	PromptGuidelines []string            `json:"promptGuidelines,omitempty"`
	PrepareArgs      func(input any) any `json:"-"`
}

type Call struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type ContentPartKind string

const (
	TextPartKind  ContentPartKind = "text"
	ImagePartKind ContentPartKind = "image"
)

type ContentPart struct {
	Kind     ContentPartKind `json:"kind"`
	Text     string          `json:"text,omitempty"`
	MIMEType string          `json:"mimeType,omitempty"`
	Data     string          `json:"data,omitempty"`
}

func TextPart(text string) ContentPart {
	return ContentPart{Kind: TextPartKind, Text: text}
}

func ImagePart(mimeType string, data string) ContentPart {
	return ContentPart{Kind: ImagePartKind, MIMEType: mimeType, Data: data}
}

type ReadResultDetails struct {
	Truncation *TruncationResult `json:"truncation,omitempty"`
}

type EditResultDetails struct {
	Diff             string `json:"diff"`
	FirstChangedLine int    `json:"firstChangedLine"`
}

type BashResultDetails struct {
	ExitCode       int    `json:"exitCode"`
	TimedOut       bool   `json:"timedOut,omitempty"`
	Cancelled      bool   `json:"cancelled,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
	FullOutputPath string `json:"fullOutputPath,omitempty"`
}

type GrepResultDetails struct {
	MatchLimitReached int               `json:"matchLimitReached,omitempty"`
	Truncation        *TruncationResult `json:"truncation,omitempty"`
	LinesTruncated    bool              `json:"linesTruncated,omitempty"`
}

type FindResultDetails struct {
	ResultLimitReached int               `json:"resultLimitReached,omitempty"`
	Truncation         *TruncationResult `json:"truncation,omitempty"`
}

type LsResultDetails struct {
	EntryLimitReached int               `json:"entryLimitReached,omitempty"`
	Truncation        *TruncationResult `json:"truncation,omitempty"`
}

type Result struct {
	CallID  string        `json:"id"`
	Name    string        `json:"name"`
	Parts   []ContentPart `json:"parts"`
	IsError bool          `json:"isError"`
	Details any           `json:"details,omitempty"`
}

func TextResult(call Call, text string) Result {
	return Result{
		CallID: call.ID,
		Name:   call.Name,
		Parts:  []ContentPart{TextPart(text)},
	}
}

func ErrorTextResult(call Call, err error) Result {
	return Result{
		CallID:  call.ID,
		Name:    call.Name,
		Parts:   []ContentPart{TextPart(err.Error())},
		IsError: true,
	}
}

func (r Result) Text() string {
	var out strings.Builder
	for _, part := range r.Parts {
		if part.Kind == TextPartKind {
			out.WriteString(part.Text)
		}
	}
	return out.String()
}

func (r Result) FirstImage() (ContentPart, bool) {
	for _, part := range r.Parts {
		if part.Kind == ImagePartKind {
			return part, true
		}
	}
	return ContentPart{}, false
}

type Tool interface {
	Definition() Definition
	Run(ctx context.Context, call Call) (Result, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Definition().Name] = tool
}

func (r *Registry) Definitions() []Definition {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}

func (r *Registry) Run(ctx context.Context, call Call) (Result, error) {
	logger := logx.FromContext(ctx).With().
		Str("component", "tool").
		Str("event", "tool.run").
		Str("tool", call.Name).
		Str("call_id", call.ID).
		Strs("arg_keys", argKeys(call.Args)).
		Logger()
	tool, ok := r.tools[call.Name]
	if !ok {
		err := fmt.Errorf("unknown tool %q", call.Name)
		logger.Error().Err(err).Msg("")
		return ErrorTextResult(call, err), err
	}
	def := tool.Definition()
	if def.PrepareArgs != nil {
		prepared := def.PrepareArgs(call.Args)
		if args, ok := prepared.(map[string]any); ok {
			call.Args = args
		}
	}
	start := time.Now()
	result, err := tool.Run(ctx, call)
	event := logger.Info()
	if err != nil || result.IsError {
		event = logger.Error().Err(err)
	}
	event.
		Bool("is_error", result.IsError).
		Int("text_len", len(strings.TrimSpace(result.Text()))).
		Dur("duration", time.Since(start)).
		Msg("")
	return result, err
}

func argKeys(args map[string]any) []string {
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
