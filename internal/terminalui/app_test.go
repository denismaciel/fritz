package terminalui

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"fritz/internal/agent"
	"fritz/internal/tool"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestResizeInputTracksLineCountWithCap(t *testing.T) {
	model := NewModel(nil)

	model.input.SetValue("one")
	model.resizeInput()
	if model.input.Height() != 1 {
		t.Fatalf("height = %d", model.input.Height())
	}

	model.input.SetValue("one\ntwo\nthree")
	model.resizeInput()
	if model.input.Height() != 3 {
		t.Fatalf("height = %d", model.input.Height())
	}

	model.input.SetValue("1\n2\n3\n4\n5\n6\n7\n8")
	model.resizeInput()
	if model.input.Height() != 8 {
		t.Fatalf("height = %d", model.input.Height())
	}
}

func TestResizeInputTracksSoftWrappedLinesWithCap(t *testing.T) {
	model := NewModel(nil)
	model.input.SetWidth(12)

	model.input.SetValue("1234567890 1234567890 1234567890")
	model.resizeInput()
	if model.input.Height() <= 1 {
		t.Fatalf("height = %d", model.input.Height())
	}

	model.input.SetValue(strings.Repeat("1234567890 ", 20))
	model.resizeInput()
	if model.input.Height() <= 6 {
		t.Fatalf("height = %d", model.input.Height())
	}
}

func TestResizeInputCapsToAvailableTerminalHeight(t *testing.T) {
	model := NewModel(nil)
	model.height = 12
	model.input.SetWidth(12)

	model.input.SetValue(strings.Repeat("1234567890 ", 20))
	model.resizeInput()
	if model.input.Height() != 7 {
		t.Fatalf("height = %d", model.input.Height())
	}
}

func TestRenderInputBoxShowsPendingImages(t *testing.T) {
	rendered := renderInputBox("hello", []tool.ContentPart{
		tool.ImagePart("image/png", base64.StdEncoding.EncodeToString([]byte("x"))),
	})
	plain := ansi.Strip(rendered)
	for _, want := range []string{"hello", "[Image #1] image/png"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered = %q missing %q", plain, want)
		}
	}
}

func TestNewModelRemovesShortcutHelpFromPlaceholder(t *testing.T) {
	model := NewModel(nil)
	if model.input.Placeholder != "Type a prompt." {
		t.Fatalf("placeholder = %q", model.input.Placeholder)
	}
}

func TestViewShowsConfiguredModelInFooter(t *testing.T) {
	runtime := &agent.Runtime{}
	setRuntimeModelID(t, runtime, "gpt-test")
	model := NewModel(runtime)

	view := model.View()
	if !strings.Contains(view, "Model: gpt-test") {
		t.Fatalf("view = %q", view)
	}
}

func TestViewShowsRunningStateAlongsideModel(t *testing.T) {
	runtime := &agent.Runtime{}
	setRuntimeModelID(t, runtime, "gemini-test")
	model := NewModel(runtime)
	model.activeRunID = "run-1"

	view := model.View()
	if !strings.Contains(view, "Model: gemini-test | Running...") {
		t.Fatalf("view = %q", view)
	}
}

func TestRenderItemColorsToolDiffPreview(t *testing.T) {
	rendered := renderItem(Item{
		Kind:          ItemTool,
		Title:         "edit path=\"README.md\"",
		Preview:       " context\n-before\n+after",
		PreviewIsDiff: true,
	}, 40)

	plain := ansi.Strip(rendered)
	for _, want := range []string{"[tool] edit path=\"README.md\"", " context", "-before", "+after"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered = %q missing %q", plain, want)
		}
	}
	if strings.Contains(plain, "Successfully replaced") {
		t.Fatalf("rendered = %q", plain)
	}
}

func TestRenderItemLeavesNonDiffToolPreviewUnstyled(t *testing.T) {
	rendered := renderItem(Item{
		Kind:    ItemTool,
		Title:   "read path=\"README.md\"",
		Preview: "- bullet\n- bullet two",
	}, 40)

	plain := ansi.Strip(rendered)
	for _, want := range []string{"- bullet", "- bullet two"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered = %q missing %q", plain, want)
		}
	}
}

func TestSyncViewportFollowsWhenAtBottom(t *testing.T) {
	model := modelWithTranscript(8)
	model.viewport.GotoBottom()
	before := model.viewport.YOffset

	model.state = model.state.AddUserPrompt("new line")
	model.syncViewport()

	if model.viewport.YOffset <= before {
		t.Fatalf("YOffset = %d, want greater than %d", model.viewport.YOffset, before)
	}
	if !model.viewport.AtBottom() {
		t.Fatalf("viewport should stay at bottom, YOffset = %d", model.viewport.YOffset)
	}
}

func TestSyncViewportPreservesManualScrollDuringUpdates(t *testing.T) {
	model := modelWithTranscript(12)
	model.viewport.GotoBottom()
	model.viewport.PageUp()
	before := model.viewport.YOffset

	model.state = model.state.AddUserPrompt("new line")
	model.syncViewport()

	if model.viewport.YOffset != before {
		t.Fatalf("YOffset = %d, want %d", model.viewport.YOffset, before)
	}
	if model.viewport.AtBottom() {
		t.Fatal("viewport should remain pinned above bottom")
	}
}

func TestPageKeysScrollTranscript(t *testing.T) {
	model := modelWithTranscript(12)
	model.viewport.GotoBottom()
	before := model.viewport.YOffset

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)

	if model.viewport.YOffset >= before {
		t.Fatalf("pgup YOffset = %d, want less than %d", model.viewport.YOffset, before)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)

	if model.viewport.YOffset <= 0 {
		t.Fatalf("pgdown YOffset = %d, want positive", model.viewport.YOffset)
	}
}

func TestMouseWheelScrollsTranscript(t *testing.T) {
	model := modelWithTranscript(12)
	model.viewport.GotoBottom()
	before := model.viewport.YOffset

	updated, _ := model.Update(tea.MouseMsg{
		Type:   tea.MouseWheelUp,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})
	model = updated.(Model)

	if model.viewport.YOffset >= before {
		t.Fatalf("wheel up YOffset = %d, want less than %d", model.viewport.YOffset, before)
	}

	updated, _ = model.Update(tea.MouseMsg{
		Type:   tea.MouseWheelDown,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	model = updated.(Model)

	if model.viewport.YOffset <= 0 {
		t.Fatalf("wheel down YOffset = %d, want positive", model.viewport.YOffset)
	}
}

func modelWithTranscript(items int) Model {
	model := NewModel(nil)
	model.viewport.Width = 40
	model.viewport.Height = 4
	for i := range items {
		model.state = model.state.AddUserPrompt(fmt.Sprintf("line %02d", i+1))
	}
	model.syncViewportAtBottom()
	return model
}
