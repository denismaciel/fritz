package terminalui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"fritz/internal/agent"
	"fritz/internal/brand"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type agentEventMsg struct{ event agent.Event }
type runDoneMsg struct{ result agent.RunResult }
type runErrMsg struct{ err error }

type Model struct {
	runtime *agent.Runtime
	events  chan tea.Msg

	state       State
	viewport    viewport.Model
	input       textarea.Model
	width       int
	height      int
	activeRunID string
}

func NewModel(runtime *agent.Runtime) Model {
	input := textarea.New()
	input.Placeholder = "Type a prompt. Enter send. Esc cancel. Ctrl+C quit."
	input.Focus()
	input.CharLimit = 0
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.KeyMap.InsertNewline.SetEnabled(true)
	_ = input.Cursor.SetMode(cursor.CursorStatic)

	vp := viewport.New(80, 20)
	model := Model{
		runtime:  runtime,
		events:   make(chan tea.Msg, 128),
		state:    NewState(),
		viewport: vp,
		input:    input,
	}
	model.resizeInput()
	return model
}

func Run(ctx context.Context, in io.Reader, out io.Writer, runtime *agent.Runtime) error {
	model := NewModel(runtime)
	program := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithAltScreen(),
	)
	_, err := program.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
			m.state = m.state.AddUserPrompt(text)
			m.input.SetValue("")
			m.resizeInput()
			m.syncViewport()
			handle, err := m.runtime.Submit(context.Background(), agent.RunRequest{Prompt: text})
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
			m.syncViewport()
			return m, nil
		case "pgup":
			m.viewport.HalfViewUp()
			return m, nil
		case "pgdown":
			m.viewport.HalfViewDown()
			return m, nil
		}
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
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.resizeInput()
	return m, cmd
}

func (m Model) View() string {
	header := lipgloss.NewStyle().Bold(true).Render(brand.CLIName)
	footerText := "Enter send | Esc cancel/clear | Ctrl+L reset | Ctrl+C quit"
	if m.activeRunID != "" {
		footerText = "Running... Esc cancel | Ctrl+C quit"
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(footerText)
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1).
		Render(m.input.View())
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.viewport.View(),
		inputBox,
		footer,
	)
}

func (m *Model) syncViewport() {
	if m.viewport.Width == 0 {
		return
	}
	lines := make([]string, 0, len(m.state.Items))
	for _, item := range m.state.Items {
		lines = append(lines, renderItem(item, m.viewport.Width))
	}
	m.viewport.SetContent(strings.Join(lines, "\n\n"))
	m.viewport.GotoBottom()
}

func (m *Model) resizeInput() {
	lines := m.input.LineCount()
	if lines < 1 {
		lines = 1
	}
	if lines > 6 {
		lines = 6
	}
	m.input.SetHeight(lines)
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
			body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(item.Preview)
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
