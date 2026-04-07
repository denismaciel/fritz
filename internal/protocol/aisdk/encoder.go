package aisdk

import (
	"io"

	"fritz/internal/agent"
	"fritz/internal/protocol/sse"
	"fritz/internal/tool"
)

type Encoder struct {
	textStarted bool
	textID      string
}

func NewEncoder() *Encoder {
	return &Encoder{}
}

func (e *Encoder) Encode(event agent.Event) []map[string]any {
	switch event.Kind {
	case agent.EventRunStarted:
		return []map[string]any{{
			"type":      "start",
			"messageId": event.MessageID,
		}}
	case agent.EventStepStarted:
		return []map[string]any{{
			"type": "start-step",
		}}
	case agent.EventReasoningStarted:
		return []map[string]any{{
			"type": "reasoning-start",
			"id":   event.MessageID,
		}}
	case agent.EventReasoningDelta:
		return []map[string]any{{
			"type":  "reasoning-delta",
			"id":    event.MessageID,
			"delta": event.TextDelta,
		}}
	case agent.EventReasoningCompleted:
		return []map[string]any{{
			"type": "reasoning-end",
			"id":   event.MessageID,
		}}
	case agent.EventTextDelta:
		e.ensureTextID(event)
		out := make([]map[string]any, 0, 2)
		if !e.textStarted {
			out = append(out, map[string]any{
				"type": "text-start",
				"id":   e.textID,
			})
			e.textStarted = true
		}
		out = append(out, map[string]any{
			"type":  "text-delta",
			"id":    e.textID,
			"delta": event.TextDelta,
		})
		return out
	case agent.EventMessageCompleted:
		e.ensureTextID(event)
		text := ""
		if event.Message != nil {
			text = event.Message.Text()
		}
		out := make([]map[string]any, 0, 3)
		if !e.textStarted {
			out = append(out, map[string]any{
				"type": "text-start",
				"id":   e.textID,
			})
			if text != "" {
				out = append(out, map[string]any{
					"type":  "text-delta",
					"id":    e.textID,
					"delta": text,
				})
			}
		}
		out = append(out, map[string]any{
			"type": "text-end",
			"id":   e.textID,
		})
		e.textStarted = false
		return out
	case agent.EventToolCallStarted:
		return []map[string]any{{
			"type":       "tool-input-available",
			"toolCallId": event.ToolCall.ID,
			"toolName":   event.ToolCall.Name,
			"input":      event.ToolCall.Args,
		}}
	case agent.EventToolCallCompleted:
		return []map[string]any{{
			"type":       "tool-output-available",
			"toolCallId": event.ToolCall.ID,
			"output":     toolResultOutput(event.ToolResult),
		}}
	case agent.EventStepFinished:
		return []map[string]any{{
			"type": "finish-step",
		}}
	case agent.EventRunFinished:
		return []map[string]any{{
			"type": "finish",
		}}
	case agent.EventRunFailed:
		return []map[string]any{{
			"type":      "error",
			"errorText": event.Error,
		}}
	case agent.EventRunCanceled:
		return []map[string]any{{
			"type":   "abort",
			"reason": event.Error,
		}}
	default:
		return nil
	}
}

func (e *Encoder) WriteEvent(w io.Writer, event agent.Event) error {
	for _, payload := range e.Encode(event) {
		if err := sse.WriteJSON(w, payload); err != nil {
			return err
		}
	}
	return nil
}

func (e *Encoder) ensureTextID(event agent.Event) {
	if e.textID == "" {
		e.textID = event.MessageID + "-text"
	}
}

func toolResultOutput(result *tool.Result) any {
	if result == nil {
		return nil
	}
	output := map[string]any{
		"name":    result.Name,
		"isError": result.IsError,
	}
	if text := result.Text(); text != "" {
		output["text"] = text
	}
	if result.Details != nil {
		output["details"] = result.Details
	}
	return output
}
