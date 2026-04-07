package agui

import (
	"io"

	"fritz/internal/agent"
	"fritz/internal/protocol/sse"
)

type Encoder struct {
	textStarted bool
}

func NewEncoder() *Encoder {
	return &Encoder{}
}

func (e *Encoder) Encode(event agent.Event) []map[string]any {
	switch event.Kind {
	case agent.EventRunStarted:
		return []map[string]any{{
			"type":        "RUN_STARTED",
			"runId":       event.RunID,
			"threadId":    event.Session.ID,
			"sessionPath": event.Session.Path,
			"messageId":   event.MessageID,
		}}
	case agent.EventStepStarted:
		return []map[string]any{{
			"type":  "STEP_STARTED",
			"runId": event.RunID,
			"step":  event.Step,
		}}
	case agent.EventReasoningStarted:
		return []map[string]any{
			{
				"type":  "REASONING_START",
				"runId": event.RunID,
			},
			{
				"type":      "REASONING_MESSAGE_START",
				"messageId": event.MessageID,
			},
		}
	case agent.EventReasoningDelta:
		return []map[string]any{{
			"type":      "REASONING_MESSAGE_CONTENT",
			"messageId": event.MessageID,
			"delta":     event.TextDelta,
		}}
	case agent.EventReasoningCompleted:
		return []map[string]any{
			{
				"type":      "REASONING_MESSAGE_END",
				"messageId": event.MessageID,
			},
			{
				"type":  "REASONING_END",
				"runId": event.RunID,
			},
		}
	case agent.EventTextDelta:
		out := make([]map[string]any, 0, 2)
		if !e.textStarted {
			out = append(out, map[string]any{
				"type":      "TEXT_MESSAGE_START",
				"messageId": event.MessageID,
			})
			e.textStarted = true
		}
		out = append(out, map[string]any{
			"type":      "TEXT_MESSAGE_CONTENT",
			"messageId": event.MessageID,
			"delta":     event.TextDelta,
		})
		return out
	case agent.EventMessageCompleted:
		text := ""
		if event.Message != nil {
			text = event.Message.Text()
		}
		out := make([]map[string]any, 0, 3)
		if !e.textStarted {
			out = append(out, map[string]any{
				"type":      "TEXT_MESSAGE_START",
				"messageId": event.MessageID,
			})
			if text != "" {
				out = append(out, map[string]any{
					"type":      "TEXT_MESSAGE_CONTENT",
					"messageId": event.MessageID,
					"delta":     text,
				})
			}
		}
		out = append(out, map[string]any{
			"type":      "TEXT_MESSAGE_END",
			"messageId": event.MessageID,
			"text":      text,
		})
		e.textStarted = false
		return out
	case agent.EventToolCallStarted:
		return []map[string]any{
			{
				"type":       "TOOL_CALL_START",
				"toolCallId": event.ToolCall.ID,
				"toolName":   event.ToolCall.Name,
			},
			{
				"type":       "TOOL_CALL_ARGS",
				"toolCallId": event.ToolCall.ID,
				"args":       event.ToolCall.Args,
			},
			{
				"type":       "TOOL_CALL_END",
				"toolCallId": event.ToolCall.ID,
			},
		}
	case agent.EventToolCallCompleted:
		return []map[string]any{{
			"type":       "TOOL_CALL_RESULT",
			"toolCallId": event.ToolCall.ID,
			"toolName":   event.ToolCall.Name,
			"result":     event.ToolResult,
		}}
	case agent.EventStepFinished:
		return []map[string]any{{
			"type":  "STEP_FINISHED",
			"runId": event.RunID,
			"step":  event.Step,
		}}
	case agent.EventRunFinished:
		return []map[string]any{{
			"type":  "RUN_FINISHED",
			"runId": event.RunID,
		}}
	case agent.EventRunFailed:
		return []map[string]any{{
			"type":    "RUN_ERROR",
			"runId":   event.RunID,
			"message": event.Error,
		}}
	case agent.EventRunCanceled:
		return []map[string]any{{
			"type":    "RUN_ERROR",
			"runId":   event.RunID,
			"code":    "canceled",
			"message": event.Error,
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
