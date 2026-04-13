package terminalui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fritz/internal/agent"
	"fritz/internal/tool"
)

type ItemKind string

const (
	ItemUser      ItemKind = "user"
	ItemAssistant ItemKind = "assistant"
	ItemReasoning ItemKind = "reasoning"
	ItemTool      ItemKind = "tool"
	ItemStatus    ItemKind = "status"
)

type Item struct {
	ID            string
	Kind          ItemKind
	Title         string
	Text          string
	Preview       string
	PreviewIsDiff bool
	Error         string
	Done          bool
}

type State struct {
	Items []Item
	index map[string]int
}

func NewState() State {
	return State{index: map[string]int{}}
}

func (s State) AddUserPrompt(text string) State {
	return s.AddUserPromptWithImages(text, nil)
}

func (s State) AddUserPromptWithImages(text string, images []tool.ContentPart) State {
	next := s.clone()
	if len(images) > 0 {
		labels := make([]string, 0, len(images))
		for i, image := range images {
			labels = append(labels, fmt.Sprintf("[Image #%d] %s", i+1, image.MIMEType))
		}
		if text != "" {
			text += "\n"
		}
		text += strings.Join(labels, "\n")
	}
	next.append(Item{
		ID:   fmt.Sprintf("user-%03d", len(next.Items)+1),
		Kind: ItemUser,
		Text: text,
		Done: true,
	})
	return next
}

func (s State) Apply(event agent.Event) State {
	next := s.clone()
	switch event.Kind {
	case agent.EventReasoningStarted:
		next.upsert(Item{
			ID:    event.MessageID,
			Kind:  ItemReasoning,
			Title: "reasoning",
		})
	case agent.EventReasoningDelta:
		item := next.item(event.MessageID, ItemReasoning)
		item.Text += event.TextDelta
		next.upsert(item)
	case agent.EventReasoningCompleted:
		item := next.item(event.MessageID, ItemReasoning)
		item.Done = true
		next.upsert(item)
	case agent.EventTextDelta:
		item := next.item(event.MessageID, ItemAssistant)
		item.Text += event.TextDelta
		next.upsert(item)
	case agent.EventMessageCompleted:
		item := next.item(event.MessageID, ItemAssistant)
		if item.Text == "" && event.Message != nil {
			item.Text = event.Message.Text()
		}
		item.Done = true
		next.upsert(item)
	case agent.EventToolCallStarted:
		item := next.item(event.ToolCall.ID, ItemTool)
		item.Title = renderToolTitle(event.ToolCall)
		next.upsert(item)
	case agent.EventToolCallCompleted:
		item := next.item(event.ToolCall.ID, ItemTool)
		item.Title = renderToolTitle(event.ToolCall)
		if event.ToolResult != nil {
			item.Text = event.ToolResult.Text()
			item.Preview, item.PreviewIsDiff = previewToolResult(*event.ToolResult)
			if event.ToolResult.IsError {
				item.Error = event.ToolResult.Text()
			}
		}
		item.Done = true
		next.upsert(item)
	case agent.EventRunFailed, agent.EventRunCanceled:
		next.append(Item{
			ID:    event.ID,
			Kind:  ItemStatus,
			Title: "error",
			Text:  event.Error,
			Done:  true,
			Error: event.Error,
		})
	}
	return next
}

func (s State) Lines() []string {
	lines := make([]string, 0, len(s.Items)*2)
	for _, item := range s.Items {
		switch item.Kind {
		case ItemUser:
			lines = append(lines, "> "+item.Text)
		case ItemAssistant:
			lines = append(lines, item.Text)
		case ItemReasoning:
			lines = append(lines, "[reasoning] "+item.Text)
		case ItemTool:
			line := "[tool] " + item.Title
			if item.Preview != "" {
				line += " => " + item.Preview
			}
			lines = append(lines, line)
		case ItemStatus:
			lines = append(lines, "[error] "+item.Text)
		}
	}
	return lines
}

func (s State) clone() State {
	next := State{
		Items: append([]Item(nil), s.Items...),
		index: map[string]int{},
	}
	for key, value := range s.index {
		next.index[key] = value
	}
	return next
}

func (s *State) append(item Item) {
	s.index[item.ID] = len(s.Items)
	s.Items = append(s.Items, item)
}

func (s *State) upsert(item Item) {
	if idx, ok := s.index[item.ID]; ok {
		s.Items[idx] = item
		return
	}
	s.append(item)
}

func (s *State) item(id string, kind ItemKind) Item {
	if idx, ok := s.index[id]; ok {
		return s.Items[idx]
	}
	return Item{ID: id, Kind: kind}
}

func renderToolTitle(call *tool.Call) string {
	if call == nil {
		return "tool"
	}
	if len(call.Args) == 0 {
		return call.Name
	}
	keys := make([]string, 0, len(call.Args))
	for key := range call.Args {
		if shouldHideToolArg(call.Name, key) {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return call.Name
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, _ := json.Marshal(call.Args[key])
		parts = append(parts, fmt.Sprintf("%s=%s", key, string(value)))
	}
	return fmt.Sprintf("%s %s", call.Name, strings.Join(parts, " "))
}

func shouldHideToolArg(toolName string, argName string) bool {
	switch toolName {
	case "edit":
		return argName == "edits"
	case "write":
		return argName == "content"
	default:
		return false
	}
}

func previewToolResult(result tool.Result) (string, bool) {
	if diff := toolDiffPreview(result); diff != "" {
		return diff, true
	}
	text := strings.TrimSpace(result.Text())
	if text == "" {
		if len(result.Parts) > 0 {
			return fmt.Sprintf("%d part(s)", len(result.Parts)), false
		}
		return "", false
	}
	const maxPreview = 160
	if len(text) <= maxPreview {
		return text, false
	}
	return text[:maxPreview] + "...", false
}

func toolDiffPreview(result tool.Result) string {
	switch result.Name {
	case "edit":
		return extractDiff(result.Details)
	case "write":
		return extractDiff(result.Details)
	default:
		return ""
	}
}

func extractDiff(details any) string {
	switch details := details.(type) {
	case tool.EditResultDetails:
		return strings.TrimSpace(details.Diff)
	case *tool.EditResultDetails:
		if details == nil {
			return ""
		}
		return strings.TrimSpace(details.Diff)
	case tool.WriteResultDetails:
		return strings.TrimSpace(details.Diff)
	case *tool.WriteResultDetails:
		if details == nil {
			return ""
		}
		return strings.TrimSpace(details.Diff)
	case map[string]any:
		diff, _ := details["diff"].(string)
		return strings.TrimSpace(diff)
	default:
		return ""
	}
}
