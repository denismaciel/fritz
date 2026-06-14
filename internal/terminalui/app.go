package terminalui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"fritz/internal/agent"
	"fritz/internal/brand"
	"fritz/internal/tool"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type agentEventMsg struct{ event agent.Event }
type runDoneMsg struct{ result agent.RunResult }
type runErrMsg struct{ err error }
type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*32, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type pasteClipboardResultMsg struct {
	result clipboardPasteResult
	err    error
}

type Model struct {
	runtime Runtime
	events  chan tea.Msg

	state       State
	viewport    viewport.Model
	input       textarea.Model
	pendingImgs []tool.ContentPart
	width       int
	height      int
	activeRunID string
}

type Runtime interface {
	Submit(context.Context, agent.RunRequest) (agent.RunHandle, error)
	Reset()
	CancelRun(runID string) bool
	ModelID() string
}

func NewModel(runtime Runtime) Model {
	return NewModelWithState(runtime, NewState())
}

func NewModelWithState(runtime Runtime, state State) Model {
	input := textarea.New()
	input.Placeholder = "Type a prompt."
	input.Focus()
	input.CharLimit = 0
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.KeyMap.InsertNewline.SetEnabled(true)
	_ = input.Cursor.SetMode(cursor.CursorStatic)
	applyInputStyles(&input)

	vp := viewport.New(80, 20)
	model := Model{
		runtime:  runtime,
		events:   make(chan tea.Msg, 128),
		state:    state,
		viewport: vp,
		input:    input,
	}
	model.resizeInput()
	return model
}

func applyInputStyles(input *textarea.Model) {
	text := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("15"))
	placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Background(lipgloss.Color("15"))
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Background(lipgloss.Color("15"))
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("15"))
	style := textarea.Style{
		Base:        base,
		CursorLine:  base,
		Placeholder: placeholder,
		Prompt:      prompt,
		Text:        text,
	}
	input.FocusedStyle = style
	input.BlurredStyle = style
	input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("0"))
	input.Cursor.TextStyle = text
}

func Run(ctx context.Context, in io.Reader, out io.Writer, runtime Runtime) error {
	return RunWithState(ctx, in, out, runtime, NewState())
}

func RunWithState(ctx context.Context, in io.Reader, out io.Writer, runtime Runtime, state State) error {
	model := NewModelWithState(runtime, state)
	program := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := program.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tick()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, tick()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeInput()
		inputHeight := m.input.Height() + 2
		m.viewport.Width = msg.Width
		m.viewport.Height = max(1, msg.Height-inputHeight-2)
		m.input.SetWidth(max(20, msg.Width-2))
		m.resizeInput()
		m.syncViewport()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+v":
			if m.activeRunID != "" {
				return m, nil
			}
			return m, func() tea.Msg {
				result, err := readWaylandClipboard()
				return pasteClipboardResultMsg{result: result, err: err}
			}
		case "esc":
			if m.activeRunID != "" {
				m.runtime.CancelRun(m.activeRunID)
				return m, nil
			}
			if strings.TrimSpace(m.input.Value()) != "" {
				m.input.SetValue("")
				m.resizeInput()
				return m, nil
			}
			return m, nil
		case "enter", "ctrl+s":
			if m.activeRunID != "" {
				return m, nil
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.state = m.state.AddUserPromptWithImages(text, m.pendingImgs)
			images := append([]tool.ContentPart(nil), m.pendingImgs...)
			m.input.SetValue("")
			m.pendingImgs = nil
			m.resizeInput()
			m.syncViewportAtBottom()
			handle, err := m.runtime.Submit(context.Background(), agent.RunRequest{Prompt: text, Images: images})
			if err != nil {
				return m, sendMsg(runErrMsg{err: err})
			}
			m.activeRunID = handle.ID
			go func() {
				for event := range handle.Events {
					m.events <- agentEventMsg{event: event}
				}
				m.events <- runDoneMsg{result: <-handle.Done}
			}()
			return m, waitForAsync(m.events)
		case "ctrl+l":
			m.runtime.Reset()
			m.state = NewState()
			m.syncViewportAtBottom()
			return m, nil
		case "pgup":
			m.viewport.PageUp()
			return m, nil
		case "pgdown":
			m.viewport.PageDown()
			return m, nil
		}
	case tea.MouseMsg:
		var viewportCmd tea.Cmd
		m.viewport, viewportCmd = m.viewport.Update(msg)
		return m, viewportCmd
	case agentEventMsg:
		m.state = m.state.Apply(msg.event)
		m.syncViewport()
		return m, waitForAsync(m.events)
	case runDoneMsg:
		m.activeRunID = ""
		return m, nil
	case runErrMsg:
		m.state = m.state.Apply(agent.Event{
			ID:    "submit-error",
			Kind:  agent.EventRunFailed,
			Error: msg.err.Error(),
		})
		m.syncViewport()
		return m, nil
	case pasteClipboardResultMsg:
		if msg.err != nil {
			m.state = m.state.Apply(agent.Event{
				ID:    "paste-image-error",
				Kind:  agent.EventRunFailed,
				Error: msg.err.Error(),
			})
		} else if msg.result.image.Kind == tool.ImagePartKind {
			m.pendingImgs = append(m.pendingImgs, msg.result.image)
		} else if msg.result.text != "" {
			m.input.InsertString(msg.result.text)
		}
		m.resizeInput()
		m.syncViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.resizeInput()
	return m, cmd
}

func (m Model) View() string {
	header := lipgloss.NewStyle().Bold(true).Render(brand.CLIName)
	footerText := "Model: unknown"
	if m.runtime != nil {
		footerText = "Model: " + m.runtime.ModelID()
	}
	if m.activeRunID != "" {
		footerText += " | " + ActivityIndicator(time.Now())
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(footerText)
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("15")).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1).
		Render(renderInputBox(m.input.View(), m.pendingImgs))
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.viewport.View(),
		inputBox,
		footer,
	)
}

func (m *Model) syncViewport() {
	m.syncViewportWithFollow(m.viewport.AtBottom())
}

func (m *Model) syncViewportAtBottom() {
	m.syncViewportWithFollow(true)
}

func (m *Model) syncViewportWithFollow(followBottom bool) {
	if m.viewport.Width == 0 {
		return
	}
	lines := make([]string, 0, len(m.state.Items))
	for _, item := range m.state.Items {
		lines = append(lines, renderItem(item, m.viewport.Width))
	}
	m.viewport.SetContent(strings.Join(lines, "\n\n"))
	if followBottom {
		m.viewport.GotoBottom()
	}
}

func (m *Model) resizeInput() {
	lines := wrappedInputLineCount(m.input)
	lines += len(m.pendingImgs)
	if lines < 1 {
		lines = 1
	}
	if maxLines := m.maxInputLines(); maxLines > 0 && lines > maxLines {
		lines = maxLines
	}
	m.input.SetHeight(lines)
}

func wrappedInputLineCount(input textarea.Model) int {
	width := input.Width()
	if width <= 0 {
		return max(1, input.LineCount())
	}
	total := 0
	for _, line := range strings.Split(input.Value(), "\n") {
		if line == "" {
			total++
			continue
		}
		wrapped := ansi.Hardwrap(ansi.Wordwrap(line, width, ""), width, true)
		count := len(strings.Split(wrapped, "\n"))
		if count < 1 {
			count = 1
		}
		total += count
	}
	if total < 1 {
		return 1
	}
	return total
}

func (m Model) maxInputLines() int {
	if m.height <= 0 {
		return 0
	}
	// header + viewport + input border + footer
	maxLines := m.height - 5
	if maxLines < 1 {
		return 1
	}
	return maxLines
}

func renderInputBox(input string, images []tool.ContentPart) string {
	if len(images) == 0 {
		return input
	}
	parts := []string{input}
	for i, image := range images {
		label := fmt.Sprintf("[Image #%d] %s", i+1, image.MIMEType)
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render(label))
	}
	return strings.Join(parts, "\n")
}

func renderItem(item Item, width int) string {
	contentWidth := max(12, width)
	switch item.Kind {
	case ItemUser:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Width(contentWidth).Render("> " + item.Text)
	case ItemAssistant:
		return lipgloss.NewStyle().Width(contentWidth).Render(item.Text)
	case ItemReasoning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(contentWidth).Render("[reasoning] " + item.Text)
	case ItemTool:
		body := item.Title
		if item.Preview != "" {
			body += "\n" + renderToolPreview(item.Preview, item.PreviewIsDiff)
		}
		if item.Error != "" {
			body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(item.Error)
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Width(contentWidth).Render("[tool] " + body)
	case ItemStatus:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Width(contentWidth).Render("[error] " + item.Text)
	default:
		return lipgloss.NewStyle().Width(contentWidth).Render(item.Text)
	}
}

func waitForAsync(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func sendMsg(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) String() string {
	return fmt.Sprintf("items=%d", len(m.state.Items))
}

func renderToolPreview(preview string, isDiff bool) string {
	if !isDiff {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(preview)
	}
	lines := strings.Split(preview, "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		switch {
		case strings.HasPrefix(line, "+"):
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		case strings.HasPrefix(line, "-"):
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		}
		rendered = append(rendered, style.Render(line))
	}
	return strings.Join(rendered, "\n")
}
