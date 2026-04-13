package chat

import (
	"strings"

	"fritz/internal/model"
	"fritz/internal/tool"
)

type Turn struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

type Transcript []Turn

func (t Transcript) BuildPrompt(input string) string {
	if len(t) == 0 {
		return input
	}

	var prompt strings.Builder
	prompt.WriteString("Conversation so far:\n")
	for _, turn := range t {
		prompt.WriteString("User: ")
		prompt.WriteString(turn.User)
		prompt.WriteString("\n")
		prompt.WriteString("Assistant: ")
		prompt.WriteString(turn.Assistant)
		prompt.WriteString("\n")
	}
	prompt.WriteString("User: ")
	prompt.WriteString(input)
	return prompt.String()
}

type InputKind string

const (
	InputEmpty  InputKind = "empty"
	InputPrompt InputKind = "prompt"
	InputHelp   InputKind = "help"
	InputReset  InputKind = "reset"
	InputQuit   InputKind = "quit"
)

type Input struct {
	Kind InputKind
	Text string
}

func ParseInput(line string) Input {
	text := strings.TrimSpace(line)
	switch text {
	case "":
		return Input{Kind: InputEmpty}
	case ":help":
		return Input{Kind: InputHelp}
	case ":reset":
		return Input{Kind: InputReset}
	case ":quit":
		return Input{Kind: InputQuit}
	default:
		return Input{Kind: InputPrompt, Text: text}
	}
}

type State struct {
	Transcript    Transcript
	Messages      []model.Message
	PendingPrompt string
}

func NewState() State {
	return State{}
}

type Effect interface {
	isEffect()
}

type Print struct {
	Line string
}

func (Print) isEffect() {}

type CallModel struct {
	Messages []model.Message
}

func (CallModel) isEffect() {}

type RunTool struct {
	Call tool.Call
}

func (RunTool) isEffect() {}

type Result struct {
	State   State
	Effects []Effect
	Exit    bool
}

func StartChat(state State, showHelp bool) Result {
	if !showHelp {
		return Result{State: state}
	}
	return Result{
		State: state,
		Effects: []Effect{
			Print{Line: "chat commands:"},
			Print{Line: "  :help"},
			Print{Line: "  :reset"},
			Print{Line: "  :quit"},
		},
	}
}

func SubmitPrompt(state State, prompt string) Result {
	return SubmitPromptWithImages(state, prompt, nil)
}

func SubmitPromptWithImages(state State, prompt string, images []tool.ContentPart) Result {
	next := state
	next.PendingPrompt = prompt
	next.Messages = append(next.Messages, model.MessageWithImages(model.UserRole, prompt, images))
	return Result{
		State: next,
		Effects: []Effect{
			CallModel{Messages: append([]model.Message(nil), next.Messages...)},
		},
	}
}

func HandleInput(state State, line string) Result {
	input := ParseInput(line)
	switch input.Kind {
	case InputEmpty:
		return Result{State: state}
	case InputHelp:
		return StartChat(state, true)
	case InputReset:
		next := state
		next.Transcript = nil
		next.Messages = nil
		next.PendingPrompt = ""
		return Result{
			State: next,
			Effects: []Effect{
				Print{Line: "history cleared"},
			},
		}
	case InputQuit:
		return Result{State: state, Exit: true}
	case InputPrompt:
		return SubmitPrompt(state, input.Text)
	default:
		return Result{State: state}
	}
}

func ApplyModelResponse(state State, response model.Response) State {
	next := state
	if len(response.Message.Parts) > 0 {
		next.Messages = append(next.Messages, response.Message)
	}
	if len(response.ToolCalls) == 0 && next.PendingPrompt != "" {
		next.Transcript = append(next.Transcript, Turn{
			User:      next.PendingPrompt,
			Assistant: response.Text,
		})
		next.PendingPrompt = ""
	}
	return next
}

func HandleModelResponse(state State, response model.Response) Result {
	next := ApplyModelResponse(state, response)
	if len(response.ToolCalls) > 0 {
		effects := make([]Effect, 0, len(response.ToolCalls))
		for _, call := range response.ToolCalls {
			effects = append(effects, RunTool{Call: call})
		}
		return Result{State: next, Effects: effects}
	}
	return Result{
		State: next,
		Effects: []Effect{
			Print{Line: response.Text},
		},
	}
}

func HandleToolResult(state State, result tool.Result) Result {
	next := state
	next.Messages = append(next.Messages, model.Message{
		Role: model.UserRole,
		Parts: []model.Part{
			{
				ToolResult: &result,
			},
		},
	})
	return Result{
		State: next,
		Effects: []Effect{
			CallModel{Messages: append([]model.Message(nil), next.Messages...)},
		},
	}
}
